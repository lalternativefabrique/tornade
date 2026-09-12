use anyhow::Result;

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

const KNEE: f32 = 0.7;

/// The model speaks around -21 LUFS with peaks near -6 dBFS, quieter than
/// spoken audio is mastered to; `gain` lifts it and a soft knee above 0.7
/// keeps the loudest peaks from clipping.
pub fn to_i16(samples: &[f32], gain: f32) -> Vec<i16> {
    samples
        .iter()
        .map(|s| {
            let x = s * gain;
            let mag = x.abs();
            let limited = if mag > KNEE {
                KNEE + (1.0 - KNEE) * ((mag - KNEE) / (1.0 - KNEE)).tanh()
            } else {
                mag
            };
            (limited.copysign(x) * 32767.0) as i16
        })
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
    crate::lame::encode_mono(pcm, sample_rate, 64, 5)
}

const LEAD_KEEP_S: f32 = 0.25;
const LEAD_SILENCE_DB: f32 = -40.0;

/// The model opens with a variable stretch of near-silence; a quarter second
/// of it is kept so the voice does not start abruptly.
pub fn trim_leading_silence(pcm: Vec<i16>, sample_rate: u32) -> Vec<i16> {
    let hop = (sample_rate / 100) as usize;
    let threshold = 32767.0 * 10f32.powf(LEAD_SILENCE_DB / 20.0);
    let first_loud = pcm.chunks(hop).position(|c| {
        let energy = c.iter().map(|&s| (s as f32) * (s as f32)).sum::<f32>();
        (energy / c.len() as f32).sqrt() > threshold
    });
    let keep = (LEAD_KEEP_S * sample_rate as f32) as usize;
    match first_loud.map(|i| i * hop) {
        Some(start) if start > keep => pcm[start - keep..].to_vec(),
        _ => pcm,
    }
}
