pub mod check;
pub mod sink;
pub mod version;

pub use anyhow::{anyhow, bail, Result};
pub type GenericError = anyhow::Error;
