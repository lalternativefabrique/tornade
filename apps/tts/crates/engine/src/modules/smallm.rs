//! Linear layers for a handful of rows. A generic GEMM packs and threads for
//! large tiles; with a few rows against hundreds of megabytes of weights the
//! cost is reading the weights, so each weight row is read once and dotted
//! with every input row while it sits in L1.

use candle_core::{CpuStorage, Result, Storage, Tensor};
use candle_nn::Linear;
use rayon::prelude::*;

const ROWS_PER_TASK: usize = 16;

fn f32_data<'a>(t: &'a Tensor, guard: &'a Storage) -> Result<&'a [f32]> {
    let layout = t.layout();
    if !layout.is_contiguous() {
        return Err(candle_core::Error::Msg("smallm: expected a contiguous tensor".into()));
    }
    match guard {
        Storage::Cpu(CpuStorage::F32(data)) => {
            Ok(&data[layout.start_offset()..layout.start_offset() + t.elem_count()])
        }
        _ => Err(candle_core::Error::Msg("smallm: expected an f32 CPU tensor".into())),
    }
}

#[inline]
fn dot(a: &[f32], b: &[f32]) -> f32 {
    let mut acc = [0f32; 16];
    let mut ca = a.chunks_exact(16);
    let mut cb = b.chunks_exact(16);
    for (xa, xb) in (&mut ca).zip(&mut cb) {
        for i in 0..16 {
            acc[i] += xa[i] * xb[i];
        }
    }
    let tail: f32 = ca.remainder().iter().zip(cb.remainder()).map(|(x, y)| x * y).sum();
    acc.iter().sum::<f32>() + tail
}

/// `x` is [M, K] and `w` is [N, K]; returns `x · wᵀ (+ bias)` as [M, N].
pub fn linear_rows(x: &Tensor, w: &Tensor, bias: Option<&Tensor>) -> Result<Tensor> {
    let (m, k) = x.dims2()?;
    let (n, kw) = w.dims2()?;
    if k != kw {
        return Err(candle_core::Error::Msg(format!("smallm: x has {k} columns, w has {kw}")));
    }
    let x = x.contiguous()?;
    let (xs, _) = x.storage_and_layout();
    let (ws, _) = w.storage_and_layout();
    let xd = f32_data(&x, &xs)?;
    let wd = f32_data(w, &ws)?;
    let bias_vec = match bias {
        Some(b) => Some(b.flatten_all()?.to_vec1::<f32>()?),
        None => None,
    };

    let mut out_t = vec![0f32; n * m];
    out_t
        .par_chunks_mut(ROWS_PER_TASK * m)
        .enumerate()
        .for_each(|(task, block)| {
            let n0 = task * ROWS_PER_TASK;
            for (i, cell) in block.chunks_mut(m).enumerate() {
                let row = &wd[(n0 + i) * k..(n0 + i + 1) * k];
                let b = bias_vec.as_ref().map_or(0.0, |v| v[n0 + i]);
                for (j, o) in cell.iter_mut().enumerate() {
                    *o = dot(row, &xd[j * k..(j + 1) * k]) + b;
                }
            }
        });
    Tensor::from_vec(out_t, (n, m), x.device())?.t()?.contiguous()
}

/// Applies `layer` to `x` of shape [M, K] or [M, 1, K].
pub fn linear(layer: &Linear, x: &Tensor) -> Result<Tensor> {
    match x.rank() {
        2 => linear_rows(x, layer.weight(), layer.bias()),
        3 => {
            let (b, t, k) = x.dims3()?;
            let out = linear_rows(&x.reshape((b * t, k))?, layer.weight(), layer.bias())?;
            let n = out.dim(1)?;
            out.reshape((b, t, n))
        }
        r => Err(candle_core::Error::Msg(format!("smallm: unsupported rank {r}"))),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use candle_core::{DType, Device, Module};

    #[test]
    fn matches_candle_linear() -> Result<()> {
        let dev = Device::Cpu;
        let w = Tensor::randn(0f32, 1.0, (40, 72), &dev)?;
        let b = Tensor::randn(0f32, 1.0, 40, &dev)?;
        let x = Tensor::randn(0f32, 1.0, (5, 72), &dev)?;
        let layer = Linear::new(w, Some(b));
        let want = layer.forward(&x)?;
        let got = linear(&layer, &x)?;
        let diff = (want - got)?.abs()?.max_all()?.to_dtype(DType::F32)?.to_scalar::<f32>()?;
        assert!(diff < 1e-4, "max diff {diff}");
        Ok(())
    }
}
