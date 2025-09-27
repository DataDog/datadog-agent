use serde_yaml::Value;
use serde::de::DeserializeOwned;

use std::collections::HashMap;
use std::ffi::{c_char, CStr};
use std::error::Error;

/// Represents the parameters passed by the Agent to the check
/// 
/// It stores every parameter in a map using `Serde` and provide a method for retrieving the values
#[repr(C)]
pub struct Config {
    map: HashMap<String, Value>,
}

impl Config {
    // NOTE: should it be moved to the test module?
    pub fn new() -> Self {
        Self { map: HashMap::new() }
    }

    pub fn from_yaml_str(cstr: *const c_char) -> Result<Self, Box<dyn Error>> {
        // read string
        let str = unsafe { CStr::from_ptr(cstr) }.to_str()?;

        // create map of parameters from the string
        match serde_yaml::from_str(str) {
            Ok(map) => Ok(Self { map }),
            Err(e) => Err(e.to_string().into()),
        }
    }

    pub fn get<T>(&self, key: &str) -> Result<T, Box<dyn Error>>
    where 
        T: DeserializeOwned,
    {
        match self.map.get(key) {
            Some(serde_value) => {
                let value = serde_yaml::from_value(serde_value.clone())?;
                Ok(value)
            },
            None => Err(format!("key '{key}' not found in the instance").into()),
        }
    }
}
