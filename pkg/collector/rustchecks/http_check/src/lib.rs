pub mod check;
pub mod version;
pub mod config;

pub use anyhow::{Result, anyhow, bail};
pub type GenericError = anyhow::Error;

pub use self::check::HttpCheck;
