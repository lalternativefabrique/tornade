use crate::ModelState;
use crate::modules::rope::RotaryEmbedding;
use crate::voice_state::{
    ATTN_K_BUF_KEY, ATTN_LEN_KEY, ATTN_POS_KEY, ATTN_V_BUF_KEY, AttentionCursor,
    read_attention_cursor, write_attention_cursor,
};
use candle_core::{DType, Result, Tensor};
use candle_nn::{Module, VarBuilder};

use crate::modules::smallm::Proj;
use std::collections::HashMap;

fn ring_chunks(buf: &Tensor, head: usize, len: usize) -> Result<Vec<Tensor>> {
    let cap = buf.dim(2)?;
    if cap == 0 {
        return Ok(Vec::new());
    }

    let len = len.min(cap);
    if len == 0 {
        return Ok(Vec::new());
    }

    let head = head % cap;
    let first_len = std::cmp::min(len, cap - head);
    let second_len = len - first_len;
    let mut chunks = Vec::with_capacity(if second_len > 0 { 2 } else { 1 });
    chunks.push(buf.narrow(2, head, first_len)?);
    if second_len > 0 {
        chunks.push(buf.narrow(2, 0, second_len)?);
    }
    Ok(chunks)
}

#[derive(Clone)]
pub struct StreamingMultiheadAttention {
    embed_dim: usize,
    num_heads: usize,
    rope: RotaryEmbedding,
    in_proj: Proj,
    out_proj: Proj,
    context: Option<usize>,
    name: String,
}

impl StreamingMultiheadAttention {
    pub fn new(
        embed_dim: usize,
        num_heads: usize,
        rope: RotaryEmbedding,
        context: Option<usize>,
        name: &str,
        vb: VarBuilder,
    ) -> Result<Self> {
        // out_dim = embed_dim + 2 * kv_dim (GQA/MHA logic in original)
        // Original code:
        // out_dim = embed_dim
        // num_kv = num_heads
        // kv_dim = (embed_dim // num_heads) * num_kv -> so embed_dim
        // out_dim += 2 * kv_dim -> so 3 * embed_dim
        let in_proj = Proj::new(candle_nn::linear_no_bias(embed_dim, 3 * embed_dim, vb.pp("in_proj"))?);
        let out_proj = Proj::new(candle_nn::linear_no_bias(embed_dim, embed_dim, vb.pp("out_proj"))?);

        Ok(Self {
            embed_dim,
            num_heads,
            rope,
            in_proj,
            out_proj,
            context,
            name: name.to_string(),
        })
    }

    pub fn quantize(&mut self) -> Result<()> {
        self.in_proj.quantize()?;
        self.out_proj.quantize()
    }

    pub fn init_state(
        &self,
        batch_size: usize,
        _sequence_length: usize,
        device: &candle_core::Device,
    ) -> Result<HashMap<String, Tensor>> {
        let dim_per_head = self.embed_dim / self.num_heads;
        let mut state = HashMap::new();

        // Initial capacity: match context if windowed, otherwise reasonable default
        let cap = self.context.unwrap_or(64);
        state.insert(
            ATTN_K_BUF_KEY.to_string(),
            Tensor::zeros(
                (batch_size, self.num_heads, cap, dim_per_head),
                DType::F32,
                device,
            )?,
        );
        state.insert(
            ATTN_V_BUF_KEY.to_string(),
            Tensor::zeros(
                (batch_size, self.num_heads, cap, dim_per_head),
                DType::F32,
                device,
            )?,
        );
        write_attention_cursor(&mut state, AttentionCursor::default(), device)?;
        Ok(state)
    }

    pub fn forward(
        &self,
        query: &Tensor,
        model_state: &mut ModelState,
        current_pos: usize,
        current_len: usize,
    ) -> Result<Tensor> {
        let (b, t, _) = query.dims3()?;
        let d = self.embed_dim / self.num_heads;
        let window_size = self.context;

        // Auto-initialize state if missing
        if !model_state.contains_key(&self.name) {
            model_state.insert(self.name.clone(), self.init_state(b, 0, query.device())?);
        }

        let module_state = model_state.get_mut(&self.name).unwrap();
        let mut cursor = read_attention_cursor(module_state);
        if !module_state.contains_key(ATTN_POS_KEY) {
            cursor.pos = current_pos;
        }
        if !module_state.contains_key(ATTN_LEN_KEY) {
            cursor.len = current_len;
        }

        let projected = self.in_proj.forward(query)?;

        // Reshape to (b, t, 3, h, d)
        let packed = projected.reshape((b, t, 3, self.num_heads, d))?;
        let mut q = packed.narrow(2, 0, 1)?.squeeze(2)?; // (b, t, h, d)
        let mut k = packed.narrow(2, 1, 1)?.squeeze(2)?; // (b, t, h, d)
        let mut v = packed.narrow(2, 2, 1)?.squeeze(2)?; // (b, t, h, d)

        // current_pos passed as argument

        // Apply RoPE
        // RoPE expects (B, T, H, D)
        (q, k) = self.rope.forward(&q, &k, current_pos)?;

        // Transpose q, k, v to (B, H, T, D) for SDPA and KV cache
        q = q.transpose(1, 2)?;
        k = k.transpose(1, 2)?;
        v = v.transpose(1, 2)?;

        // KV cache management.
        // We take ownership from the state to avoid clones and ensure uniqueness for slice_set.
        let (mut k_buf, mut v_buf) = match (
            module_state.remove(ATTN_K_BUF_KEY),
            module_state.remove(ATTN_V_BUF_KEY),
        ) {
            (Some(kb), Some(vb)) => (kb, vb),
            _ => {
                let initial_cap = window_size.unwrap_or(64);
                let kb = Tensor::zeros((b, self.num_heads, initial_cap, d), q.dtype(), q.device())?;
                let vb = Tensor::zeros((b, self.num_heads, initial_cap, d), q.dtype(), q.device())?;
                (kb, vb)
            }
        };

        let mut cap = k_buf.dim(2)?; // Current capacity of the buffer
        let mut cache_len = cursor.len.min(cap);
        let mut cache_head = if cap > 0 { cursor.head % cap } else { 0 };

        let x = if let Some(window_size) = self.context {
            // Ensure fixed ring capacity for windowed attention.
            if cap != window_size {
                if cap > window_size {
                    k_buf = k_buf.narrow(2, 0, window_size)?.contiguous()?;
                    v_buf = v_buf.narrow(2, 0, window_size)?.contiguous()?;
                } else {
                    let zeros_shape = (b, self.num_heads, window_size - cap, d);
                    let k_zeros = Tensor::zeros(zeros_shape, q.dtype(), q.device())?;
                    let v_zeros = Tensor::zeros(zeros_shape, q.dtype(), q.device())?;
                    k_buf = Tensor::cat(&[k_buf, k_zeros], 2)?;
                    v_buf = Tensor::cat(&[v_buf, v_zeros], 2)?;
                }
                cap = window_size;
                cache_len = cache_len.min(cap);
                cache_head = 0;
            }

            // Build chronological KV chunks from ring cache + current K/V chunk.
            let mut k_chunks = ring_chunks(&k_buf, cache_head, cache_len)?;
            let mut v_chunks = ring_chunks(&v_buf, cache_head, cache_len)?;
            k_chunks.push(k.clone());
            v_chunks.push(v.clone());

            let scale = 1.0 / (d as f64).sqrt();
            if k_chunks.len() == 1 {
                crate::modules::sdpa::sdpa(
                    &q,
                    &k_chunks[0],
                    &v_chunks[0],
                    scale,
                    true,
                    self.context,
                )?
            } else {
                crate::modules::sdpa::sdpa_chunked(
                    &q,
                    &k_chunks,
                    &v_chunks,
                    scale,
                    true,
                    self.context,
                )?
            }
        } else {
            // Linear attention (FlowLM) with doubling contiguous buffer.
            if cache_len + t > cap {
                let new_cap = (cache_len + t).next_power_of_two();
                let zeros_shape = (b, self.num_heads, new_cap - cap, d);
                let k_zeros = Tensor::zeros(zeros_shape, q.dtype(), q.device())?;
                let v_zeros = Tensor::zeros(zeros_shape, q.dtype(), q.device())?;
                k_buf = Tensor::cat(&[k_buf, k_zeros], 2)?;
                v_buf = Tensor::cat(&[v_buf, v_zeros], 2)?;
            }
            k_buf.slice_set(&k.contiguous()?, 2, cache_len)?;
            v_buf.slice_set(&v.contiguous()?, 2, cache_len)?;
            cache_len += t;
            cache_head = 0;

            // Get current KV for attention
            let kc = k_buf.narrow(2, 0, cache_len)?;
            let vc = v_buf.narrow(2, 0, cache_len)?;
            let scale = 1.0 / (d as f64).sqrt();
            crate::modules::sdpa::sdpa(&q, &kc, &vc, scale, true, self.context)?
        };

        if let Some(window_size) = self.context {
            if t >= window_size {
                k_buf = k.narrow(2, t - window_size, window_size)?.contiguous()?;
                v_buf = v.narrow(2, t - window_size, window_size)?.contiguous()?;
                cache_head = 0;
                cache_len = window_size;
            } else if window_size > 0 {
                let evict = (cache_len + t).saturating_sub(window_size);
                if evict > 0 {
                    cache_head = (cache_head + evict) % window_size;
                    cache_len -= evict;
                }

                let write_start = (cache_head + cache_len) % window_size;
                let first = std::cmp::min(t, window_size - write_start);
                let second = t - first;

                if first > 0 {
                    let k_first = k.narrow(2, 0, first)?.contiguous()?;
                    let v_first = v.narrow(2, 0, first)?.contiguous()?;
                    k_buf.slice_set(&k_first, 2, write_start)?;
                    v_buf.slice_set(&v_first, 2, write_start)?;
                }
                if second > 0 {
                    let k_second = k.narrow(2, first, second)?.contiguous()?;
                    let v_second = v.narrow(2, first, second)?.contiguous()?;
                    k_buf.slice_set(&k_second, 2, 0)?;
                    v_buf.slice_set(&v_second, 2, 0)?;
                }
                cache_len += t;
            }
        }

        module_state.insert(ATTN_K_BUF_KEY.to_string(), k_buf);
        module_state.insert(ATTN_V_BUF_KEY.to_string(), v_buf);
        write_attention_cursor(
            module_state,
            AttentionCursor {
                pos: current_pos + t,
                len: cache_len,
                head: cache_head,
            },
            q.device(),
        )?;

        // Transpose back to [B, T, H, D] and project out
        let x = x.transpose(1, 2)?.reshape((b, t, self.embed_dim))?;
        let x = self.out_proj.forward(&x)?;

        Ok(x)
    }

    /// One frame for every row of `cache` at once: projections over the
    /// stacked rows, then a single masked attention against the stacked
    /// cache. Only the unwindowed (flow LM) path is supported.
    pub fn forward_stacked(&self, query: &Tensor, cache: &mut StackedKv) -> Result<Tensor> {
        if self.context.is_some() {
            return Err(candle_core::Error::Msg(
                "forward_stacked: windowed attention is not batched".into(),
            ));
        }
        let (b, t, _) = query.dims3()?;
        if t != 1 || b != cache.rows() {
            return Err(candle_core::Error::Msg(format!(
                "forward_stacked: query is [{b}, {t}, _] for {} rows",
                cache.rows()
            )));
        }
        let h = self.num_heads;
        let d = self.embed_dim / h;
        let packed = self.in_proj.rows(query)?.reshape((b, t, 3, h, d))?;
        let q = packed.narrow(2, 0, 1)?.squeeze(2)?;
        let k = packed.narrow(2, 1, 1)?.squeeze(2)?;
        let (q, k) = self.rope.forward_rows(&q, &k, &cache.pos)?;
        let q = q.transpose(1, 2)?.contiguous()?; // [B, H, 1, D]
        let k = k.transpose(1, 2)?.contiguous()?;
        let v = packed.narrow(2, 2, 1)?.squeeze(2)?.transpose(1, 2)?.contiguous()?;

        cache.push(&k, &v)?;
        let x = cache.attend(&q)?; // [B, H, 1, D]
        let x = x.transpose(1, 2)?.reshape((b, t, self.embed_dim))?;
        self.out_proj.rows(&x)
    }
}

fn f32_data<'a>(
    t: &'a Tensor,
    guard: &'a candle_core::Storage,
) -> Result<&'a [f32]> {
    let layout = t.layout();
    if !layout.is_contiguous() {
        return Err(candle_core::Error::Msg("expected a contiguous tensor".into()));
    }
    match guard {
        candle_core::Storage::Cpu(candle_core::CpuStorage::F32(data)) => {
            Ok(&data[layout.start_offset()..layout.start_offset() + t.elem_count()])
        }
        _ => Err(candle_core::Error::Msg("expected an f32 CPU tensor".into())),
    }
}

fn f16_data<'a>(
    t: &'a Tensor,
    guard: &'a candle_core::Storage,
) -> Result<&'a [half::f16]> {
    let layout = t.layout();
    if !layout.is_contiguous() {
        return Err(candle_core::Error::Msg("expected a contiguous tensor".into()));
    }
    match guard {
        candle_core::Storage::Cpu(candle_core::CpuStorage::F16(data)) => {
            Ok(&data[layout.start_offset()..layout.start_offset() + t.elem_count()])
        }
        _ => Err(candle_core::Error::Msg("expected an f16 CPU tensor".into())),
    }
}

/// Softmax attention of one query per (row, head) over that row's valid
/// window of the cache. One matmul per (row, head) would pay a kernel launch
/// each; this reads the window once and vectorises the head dimension.
fn stacked_attention(
    q: &[f32],
    k: &[half::f16],
    v: &[half::f16],
    (b, h, cap, d): (usize, usize, usize, usize),
    cur: usize,
    len: &[usize],
) -> Vec<f32> {
    use half::slice::HalfFloatSliceExt;
    use rayon::prelude::*;
    let scale = 1.0 / (d as f32).sqrt();
    let mut out = vec![0f32; b * h * d];
    out.par_chunks_mut(d).enumerate().for_each(|(bh, o)| {
        let row = bh / h;
        let start = cur - len[row];
        let q = &q[bh * d..(bh + 1) * d];
        let kb = &k[bh * cap * d..(bh + 1) * cap * d];
        let vb = &v[bh * cap * d..(bh + 1) * cap * d];
        let mut buf = vec![0f32; d];
        let mut scores: Vec<f32> = (start..cur)
            .map(|j| {
                kb[j * d..(j + 1) * d].convert_to_f32_slice(&mut buf);
                q.iter().zip(&buf).map(|(a, b)| a * b).sum::<f32>() * scale
            })
            .collect();
        let max = scores.iter().cloned().fold(f32::NEG_INFINITY, f32::max);
        let mut sum = 0f32;
        for s in scores.iter_mut() {
            *s = (*s - max).exp();
            sum += *s;
        }
        let inv = 1.0 / sum;
        for (w, j) in scores.iter().zip(start..cur) {
            let w = w * inv;
            vb[j * d..(j + 1) * d].convert_to_f32_slice(&mut buf);
            for (o, x) in o.iter_mut().zip(&buf) {
                *o += w * x;
            }
        }
    });
    out
}

/// Key/value cache of one attention layer for several streams stacked on
/// the batch dimension. Rows are right-aligned on `cur`: row `i` occupies
/// columns `[cur - len[i], cur)`, so every row writes the same column on
/// each step and one mask hides the rest.
#[derive(Clone)]
pub struct StackedKv {
    pub k: Tensor,
    pub v: Tensor,
    pub cur: usize,
    pub len: Vec<usize>,
    pub pos: Vec<usize>,
}

impl StackedKv {
    pub fn empty(num_heads: usize, head_dim: usize, device: &candle_core::Device) -> Result<Self> {
        Ok(Self {
            k: Tensor::zeros((0, num_heads, 0, head_dim), DType::F16, device)?,
            v: Tensor::zeros((0, num_heads, 0, head_dim), DType::F16, device)?,
            cur: 0,
            len: Vec::new(),
            pos: Vec::new(),
        })
    }

    pub fn rows(&self) -> usize {
        self.len.len()
    }

    fn cap(&self) -> Result<usize> {
        self.k.dim(2)
    }

    /// Appends one row whose cache is `k`/`v` of shape [1, H, len, D] with
    /// RoPE already applied, continuing at position `pos`.
    pub fn add_row(&mut self, k: &Tensor, v: &Tensor, pos: usize) -> Result<()> {
        let k = &k.to_dtype(DType::F16)?;
        let v = &v.to_dtype(DType::F16)?;
        let (_, h, len, d) = k.dims4()?;
        let device = k.device();
        let new_cur = self.cur.max(len);
        let new_cap = (new_cur + 1).max(self.cap()?).next_power_of_two();
        let rows = self.rows();

        let place = |t: &Tensor, left: usize, right: usize| -> Result<Tensor> {
            let n = t.dim(0)?;
            let mut parts = Vec::with_capacity(3);
            if left > 0 {
                parts.push(Tensor::zeros((n, h, left, d), DType::F16, device)?);
            }
            parts.push(t.clone());
            if right > 0 {
                parts.push(Tensor::zeros((n, h, right, d), DType::F16, device)?);
            }
            Tensor::cat(&parts, 2)
        };
        let shift = new_cur - self.cur;
        let mut ks = Vec::with_capacity(2);
        let mut vs = Vec::with_capacity(2);
        if rows > 0 {
            let old_cap = self.cap()?;
            ks.push(place(&self.k, shift, new_cap - old_cap - shift)?);
            vs.push(place(&self.v, shift, new_cap - old_cap - shift)?);
        }
        ks.push(place(k, new_cur - len, new_cap - new_cur)?);
        vs.push(place(v, new_cur - len, new_cap - new_cur)?);
        self.k = Tensor::cat(&ks, 0)?.contiguous()?;
        self.v = Tensor::cat(&vs, 0)?.contiguous()?;
        self.cur = new_cur;
        self.len.push(len);
        self.pos.push(pos);
        Ok(())
    }

    pub fn remove_row(&mut self, i: usize) -> Result<()> {
        let keep: Vec<u32> = (0..self.rows() as u32).filter(|&r| r as usize != i).collect();
        let idx = Tensor::new(keep, self.k.device())?;
        self.k = self.k.index_select(&idx, 0)?.contiguous()?;
        self.v = self.v.index_select(&idx, 0)?.contiguous()?;
        self.len.remove(i);
        self.pos.remove(i);
        Ok(())
    }

    fn push(&mut self, k: &Tensor, v: &Tensor) -> Result<()> {
        let (b, h, _, d) = k.dims4()?;
        if self.cur >= self.cap()? {
            let extra = self.cap()?.max(64);
            let zeros = Tensor::zeros((b, h, extra, d), DType::F16, k.device())?;
            self.k = Tensor::cat(&[&self.k, &zeros], 2)?.contiguous()?;
            self.v = Tensor::cat(&[&self.v, &zeros], 2)?.contiguous()?;
        }
        self.k.slice_set(&k.to_dtype(DType::F16)?.contiguous()?, 2, self.cur)?;
        self.v.slice_set(&v.to_dtype(DType::F16)?.contiguous()?, 2, self.cur)?;
        self.cur += 1;
        for i in 0..b {
            self.len[i] += 1;
            self.pos[i] += 1;
        }
        Ok(())
    }

    /// `q` is [B, H, 1, D]; returns [B, H, 1, D].
    fn attend(&self, q: &Tensor) -> Result<Tensor> {
        let (b, h, cap, d) = self.k.dims4()?;
        let q = q.contiguous()?;
        let (qs, _) = q.storage_and_layout();
        let (ks, _) = self.k.storage_and_layout();
        let (vs, _) = self.v.storage_and_layout();
        let out = stacked_attention(
            f32_data(&q, &qs)?,
            f16_data(&self.k, &ks)?,
            f16_data(&self.v, &vs)?,
            (b, h, cap, d),
            self.cur,
            &self.len,
        );
        Tensor::from_vec(out, (b, h, 1, d), self.k.device())
    }
}
