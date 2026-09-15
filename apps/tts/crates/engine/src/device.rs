//! Which device the model runs on, named the way a deployment names it:
//! `cpu`, `cuda` or `cuda:<index>`. CUDA needs the `cuda` build feature;
//! asking for it without one is an error, not a silent fall back to CPU.

use anyhow::{Result, bail};
use candle_core::Device;

pub fn parse(name: &str) -> Result<Device> {
    let name = name.trim().to_ascii_lowercase();
    if name.is_empty() || name == "cpu" {
        return Ok(Device::Cpu);
    }
    let Some(rest) = name.strip_prefix("cuda") else {
        bail!("unknown device {name:?}: expected cpu, cuda or cuda:<index>");
    };
    let index: usize = match rest.strip_prefix(':') {
        None if rest.is_empty() => 0,
        Some(i) => i
            .parse()
            .map_err(|_| anyhow::anyhow!("bad cuda index in {name:?}"))?,
        None => bail!("unknown device {name:?}: expected cpu, cuda or cuda:<index>"),
    };
    cuda(index)
}

#[cfg(feature = "cuda")]
fn cuda(index: usize) -> Result<Device> {
    Ok(Device::new_cuda(index)?)
}

#[cfg(not(feature = "cuda"))]
fn cuda(index: usize) -> Result<Device> {
    bail!("cuda:{index} requested but this build has no cuda feature");
}

/// The device named by `TTS_DEVICE`, cpu when unset.
pub fn from_env() -> Result<Device> {
    parse(&std::env::var("TTS_DEVICE").unwrap_or_default())
}

pub fn describe(device: &Device) -> String {
    match device {
        Device::Cpu => "cpu".into(),
        Device::Cuda(_) => "cuda".into(),
        Device::Metal(_) => "metal".into(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cpu_by_default_and_by_name() {
        assert!(parse("").unwrap().is_cpu());
        assert!(parse("cpu").unwrap().is_cpu());
        assert!(parse(" CPU ").unwrap().is_cpu());
    }

    #[test]
    fn unknown_names_are_refused() {
        assert!(parse("gpu").is_err());
        assert!(parse("cuda:x").is_err());
        assert!(parse("cudax").is_err());
    }

    #[cfg(not(feature = "cuda"))]
    #[test]
    fn cuda_without_the_feature_is_an_error_not_cpu() {
        assert!(parse("cuda").is_err());
        assert!(parse("cuda:1").is_err());
    }
}
