use anyhow::{Ok, Result, bail};

use serde::de::DeserializeOwned;
use serde_yaml::Value;

use std::collections::HashMap;

/// Represents the parameters passed by the Agent to the check
///
/// It stores every parameter in a map using `Serde` and provide a method for retrieving the values
#[repr(C)]
pub struct Config {
    map: HashMap<String, Value>,
}

impl Config {
    /// Create a configuration map base on a YAML string
    pub fn from_str(config_yaml_str: &str) -> Result<Self> {
        let map = serde_yaml::from_str(config_yaml_str)?;
        Ok(Self { map })
    }

    pub fn get<T: DeserializeOwned>(&self, key: &str) -> Result<T> {
        if let Some(serde_value) = self.map.get(key) {
            let value = serde_yaml::from_value(serde_value.clone())?;
            return Ok(value);
        }
        bail!("key '{key}' not found in config")
    }
}
