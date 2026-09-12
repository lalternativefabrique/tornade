use anyhow::{Context, Result};
use mp3lame_encoder::{Bitrate, Builder, FlushNoGap, MonoPcm, Quality};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Format {
    Mp3,
    Wav,
    Pcm,
}

impl Format {
    pub fn parse(name: &str) -> Option<Self> {
        match name {
            "mp3" | "" => Some(Self::Mp3),
            "wav" => Some(Self::Wav),
            "pcm" => Some(Self::Pcm),
            _ => None,
        }
    }

    pub fn mime(self) -> &'static str {
        match self {
            Self::Mp3 => "audio/mpeg",
            Self::Wav => "audio/wav",
            Self::Pcm => "audio/pcm",
        }
    }
}

/// The model speaks around -21 LUFS with peaks near -6 dBFS, quieter than
/// the -16 LUFS listeners expect from spoken audio; `gain` lifts it.
pub fn to_i16(samples: &[f32], gain: f32) -> Vec<i16> {
    samples
        .iter()
        .map(|s| ((s * gain).clamp(-1.0, 1.0) * 32767.0) as i16)
        .collect()
}

pub fn db_to_gain(db: f32) -> f32 {
    10f32.powf(db / 20.0)
}

pub fn encode(format: Format, pcm: &[i16], sample_rate: u32) -> Result<Vec<u8>> {
    match format {
        Format::Pcm => Ok(pcm.iter().flat_map(|s| s.to_le_bytes()).collect()),
        Format::Wav => Ok(wav(pcm, sample_rate)),
        Format::Mp3 => mp3(pcm, sample_rate),
    }
}

fn wav(pcm: &[i16], sample_rate: u32) -> Vec<u8> {
    let data_len = (pcm.len() * 2) as u32;
    let mut out = Vec::with_capacity(44 + data_len as usize);
    out.extend_from_slice(b"RIFF");
    out.extend_from_slice(&(36 + data_len).to_le_bytes());
    out.extend_from_slice(b"WAVEfmt ");
    out.extend_from_slice(&16u32.to_le_bytes());
    out.extend_from_slice(&1u16.to_le_bytes());
    out.extend_from_slice(&1u16.to_le_bytes());
    out.extend_from_slice(&sample_rate.to_le_bytes());
    out.extend_from_slice(&(sample_rate * 2).to_le_bytes());
    out.extend_from_slice(&2u16.to_le_bytes());
    out.extend_from_slice(&16u16.to_le_bytes());
    out.extend_from_slice(b"data");
    out.extend_from_slice(&data_len.to_le_bytes());
    for s in pcm {
        out.extend_from_slice(&s.to_le_bytes());
    }
    out
}

fn mp3(pcm: &[i16], sample_rate: u32) -> Result<Vec<u8>> {
    let mut builder = Builder::new().context("lame: init")?;
    builder
        .set_num_channels(1)
        .map_err(|e| anyhow::anyhow!("lame: channels: {e}"))?;
    builder
        .set_sample_rate(sample_rate)
        .map_err(|e| anyhow::anyhow!("lame: sample rate: {e}"))?;
    builder
        .set_brate(Bitrate::Kbps64)
        .map_err(|e| anyhow::anyhow!("lame: bitrate: {e}"))?;
    builder
        .set_quality(Quality::Good)
        .map_err(|e| anyhow::anyhow!("lame: quality: {e}"))?;
    let mut encoder = builder
        .build()
        .map_err(|e| anyhow::anyhow!("lame: build: {e}"))?;

    let mut out = Vec::with_capacity(mp3lame_encoder::max_required_buffer_size(pcm.len()));
    let written = encoder
        .encode(MonoPcm(pcm), out.spare_capacity_mut())
        .map_err(|e| anyhow::anyhow!("lame: encode: {e}"))?;
    unsafe { out.set_len(out.len() + written) };
    out.reserve(mp3lame_encoder::max_required_buffer_size(0) + 7200);
    let written = encoder
        .flush::<FlushNoGap>(out.spare_capacity_mut())
        .map_err(|e| anyhow::anyhow!("lame: flush: {e}"))?;
    unsafe { out.set_len(out.len() + written) };
    Ok(out)
}
