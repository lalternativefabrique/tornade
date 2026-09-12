//! The few libmp3lame calls a mono encoder needs, against the system library
//! (libmp3lame-dev at build time, libmp3lame0 at run time).

use std::ffi::c_int;

use anyhow::{Result, bail};

#[repr(C)]
struct LameGlobalFlags {
    _private: [u8; 0],
}

#[link(name = "mp3lame")]
unsafe extern "C" {
    fn lame_init() -> *mut LameGlobalFlags;
    fn lame_set_in_samplerate(gfp: *mut LameGlobalFlags, rate: c_int) -> c_int;
    fn lame_set_num_channels(gfp: *mut LameGlobalFlags, channels: c_int) -> c_int;
    fn lame_set_mode(gfp: *mut LameGlobalFlags, mode: c_int) -> c_int;
    fn lame_set_brate(gfp: *mut LameGlobalFlags, kbps: c_int) -> c_int;
    fn lame_set_quality(gfp: *mut LameGlobalFlags, quality: c_int) -> c_int;
    fn lame_init_params(gfp: *mut LameGlobalFlags) -> c_int;
    fn lame_encode_buffer(
        gfp: *mut LameGlobalFlags,
        left: *const i16,
        right: *const i16,
        samples: c_int,
        out: *mut u8,
        out_size: c_int,
    ) -> c_int;
    fn lame_encode_flush(gfp: *mut LameGlobalFlags, out: *mut u8, out_size: c_int) -> c_int;
    fn lame_close(gfp: *mut LameGlobalFlags) -> c_int;
}

const MODE_MONO: c_int = 3;

struct Encoder(*mut LameGlobalFlags);

impl Drop for Encoder {
    fn drop(&mut self) {
        unsafe { lame_close(self.0) };
    }
}

pub fn encode_mono(pcm: &[i16], sample_rate: u32, kbps: u32, quality: u32) -> Result<Vec<u8>> {
    let gfp = unsafe { lame_init() };
    if gfp.is_null() {
        bail!("lame: init failed");
    }
    let enc = Encoder(gfp);
    let ok = unsafe {
        lame_set_in_samplerate(enc.0, sample_rate as c_int) == 0
            && lame_set_num_channels(enc.0, 1) == 0
            && lame_set_mode(enc.0, MODE_MONO) == 0
            && lame_set_brate(enc.0, kbps as c_int) == 0
            && lame_set_quality(enc.0, quality as c_int) == 0
            && lame_init_params(enc.0) >= 0
    };
    if !ok {
        bail!("lame: rejected {sample_rate} Hz mono at {kbps} kbps");
    }

    let cap = pcm.len() * 5 / 4 + 7200;
    let mut out = vec![0u8; cap];
    let written = unsafe {
        lame_encode_buffer(
            enc.0,
            pcm.as_ptr(),
            pcm.as_ptr(),
            pcm.len() as c_int,
            out.as_mut_ptr(),
            cap as c_int,
        )
    };
    if written < 0 {
        bail!("lame: encode failed ({written})");
    }
    let mut len = written as usize;
    let flushed =
        unsafe { lame_encode_flush(enc.0, out[len..].as_mut_ptr(), (cap - len) as c_int) };
    if flushed < 0 {
        bail!("lame: flush failed ({flushed})");
    }
    len += flushed as usize;
    out.truncate(len);
    Ok(out)
}
