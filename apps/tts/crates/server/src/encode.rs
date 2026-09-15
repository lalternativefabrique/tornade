use anyhow::Result;

use crate::lame::Mp3Encoder;

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
/// of it is kept so the voice does not start abruptly. Samples are held back
/// until the voice is heard, then passed through untouched.
pub struct LeadTrimmer {
    hop: usize,
    keep: usize,
    threshold: f32,
    held: Vec<i16>,
    scanned: usize,
    passing: bool,
}

impl LeadTrimmer {
    pub fn new(sample_rate: u32) -> Self {
        Self {
            hop: (sample_rate / 100) as usize,
            keep: (LEAD_KEEP_S * sample_rate as f32) as usize,
            threshold: 32767.0 * 10f32.powf(LEAD_SILENCE_DB / 20.0),
            held: Vec::new(),
            scanned: 0,
            passing: false,
        }
    }

    pub fn push(&mut self, pcm: &[i16]) -> Vec<i16> {
        if self.passing {
            return pcm.to_vec();
        }
        self.held.extend_from_slice(pcm);
        let loud = self.held[self.scanned..]
            .chunks_exact(self.hop)
            .position(|c| {
                let energy = c.iter().map(|&s| (s as f32) * (s as f32)).sum::<f32>();
                (energy / c.len() as f32).sqrt() > self.threshold
            })
            .map(|i| self.scanned + i * self.hop);
        let Some(start) = loud else {
            self.scanned = self.held.len() / self.hop * self.hop;
            return Vec::new();
        };
        self.passing = true;
        let from = start.saturating_sub(self.keep);
        self.held.drain(..from);
        std::mem::take(&mut self.held)
    }

    /// What was still held back: a reading that never rose above the
    /// threshold is returned whole rather than dropped.
    pub fn finish(self) -> Vec<i16> {
        self.held
    }
}

pub fn trim_leading_silence(pcm: Vec<i16>, sample_rate: u32) -> Vec<i16> {
    let mut trimmer = LeadTrimmer::new(sample_rate);
    let mut out = trimmer.push(&pcm);
    out.extend(trimmer.finish());
    out
}

/// Encodes a reading frame by frame for a format whose bytes can be sent as
/// they are made. `None` for wav, whose header carries the total length.
pub struct Chunked {
    format: Format,
    trimmer: LeadTrimmer,
    mp3: Option<Mp3Encoder>,
}

impl Chunked {
    pub fn new(format: Format, sample_rate: u32) -> Result<Option<Self>> {
        let mp3 = match format {
            Format::Wav => return Ok(None),
            Format::Pcm => None,
            Format::Mp3 => Some(Mp3Encoder::new(sample_rate, 64, 5)?),
        };
        Ok(Some(Self {
            format,
            trimmer: LeadTrimmer::new(sample_rate),
            mp3,
        }))
    }

    pub fn push(&mut self, frame: &[i16]) -> Result<Vec<u8>> {
        let pcm = self.trimmer.push(frame);
        self.bytes(&pcm)
    }

    pub fn finish(mut self) -> Result<Vec<u8>> {
        let tail = std::mem::replace(&mut self.trimmer, LeadTrimmer::new(1)).finish();
        let mut out = self.bytes(&tail)?;
        if let Some(mp3) = self.mp3.take() {
            out.extend(mp3.finish()?);
        }
        Ok(out)
    }

    fn bytes(&mut self, pcm: &[i16]) -> Result<Vec<u8>> {
        if pcm.is_empty() {
            return Ok(Vec::new());
        }
        match (self.format, self.mp3.as_mut()) {
            (Format::Mp3, Some(enc)) => enc.encode(pcm),
            _ => Ok(pcm.iter().flat_map(|s| s.to_le_bytes()).collect()),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const RATE: u32 = 24000;

    fn silence_then_tone() -> Vec<i16> {
        let mut pcm = vec![0i16; RATE as usize];
        pcm.extend((0..RATE as usize).map(|i| {
            ((i as f32 * 440.0 * std::f32::consts::TAU / RATE as f32).sin() * 8000.0) as i16
        }));
        pcm
    }

    #[test]
    fn trimmer_matches_whole_trim_when_fed_in_frames() {
        let pcm = silence_then_tone();
        let whole = trim_leading_silence(pcm.clone(), RATE);
        let mut trimmer = LeadTrimmer::new(RATE);
        let mut framed = Vec::new();
        for frame in pcm.chunks(1920) {
            framed.extend(trimmer.push(frame));
        }
        framed.extend(trimmer.finish());
        assert_eq!(framed, whole);
        assert_eq!(whole.len(), RATE as usize + (0.25 * RATE as f32) as usize);
    }

    #[test]
    fn trimmer_keeps_a_reading_that_never_speaks() {
        let pcm = vec![0i16; 4000];
        let mut trimmer = LeadTrimmer::new(RATE);
        let mut out = trimmer.push(&pcm);
        out.extend(trimmer.finish());
        assert_eq!(out, pcm);
    }

    #[test]
    fn chunked_mp3_equals_one_shot_encode() {
        let pcm = silence_then_tone();
        let one_shot = encode(Format::Mp3, &trim_leading_silence(pcm.clone(), RATE), RATE).unwrap();
        let mut chunked = Chunked::new(Format::Mp3, RATE).unwrap().unwrap();
        let mut streamed = Vec::new();
        for frame in pcm.chunks(1920) {
            streamed.extend(chunked.push(frame).unwrap());
        }
        streamed.extend(chunked.finish().unwrap());
        assert_eq!(streamed, one_shot);
    }

    #[test]
    fn chunked_pcm_is_the_trimmed_samples() {
        let pcm = silence_then_tone();
        let mut chunked = Chunked::new(Format::Pcm, RATE).unwrap().unwrap();
        let mut streamed = Vec::new();
        for frame in pcm.chunks(1920) {
            streamed.extend(chunked.push(frame).unwrap());
        }
        streamed.extend(chunked.finish().unwrap());
        assert_eq!(
            streamed,
            encode(Format::Pcm, &trim_leading_silence(pcm, RATE), RATE).unwrap()
        );
    }

    #[test]
    fn wav_is_not_chunked() {
        assert!(Chunked::new(Format::Wav, RATE).unwrap().is_none());
    }
}
