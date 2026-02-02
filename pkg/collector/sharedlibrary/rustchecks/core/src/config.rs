use anyhow::{Result, bail};

use serde_yaml::Value;
use serde::de::DeserializeOwned;

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
    pub fn new(yaml_str: &str) -> Result<Self> {
        // try to convert the YAML string
        match serde_yaml::from_str(yaml_str) {
            Ok(map) => Ok(Self { map }),
            Err(e) => bail!(e),
        }
    }

    pub fn get<T>(&self, key: &str) -> Result<T>
    where 
        T: DeserializeOwned,
    {
        match self.map.get(key) {
            Some(serde_value) => {
                let value = serde_yaml::from_value(serde_value.clone())?;
                Ok(value)
            },
            None => bail!("key '{key}' not found in the instance"),
        }
    }
}

#[cfg(test)]
mod test {
    use std::collections::HashMap;

    use crate::config::Config;
    
    fn common_config() -> Config {
        let yaml_str = "string: \"string value\"
integer: 123456";

        Config::new(yaml_str).unwrap()
    }

    #[test]
    fn test_empty_yaml_str() {
        // should create a config even with an empty string
        let empty_config = Config::new("").unwrap();

        // the map of the config should have no keys
        assert_eq!(empty_config.map, HashMap::new());
    }

    #[test]
    fn test_exisintg_key() {
        let config = common_config();
        
        // extract config values
        let str_field: String = config.get("string").unwrap();
        let int_field: i32 = config.get("integer").unwrap();

        // verify their content
        assert_eq!(str_field, "string value");
        assert_eq!(int_field, 123456);
    }

    #[test]
    fn test_non_existing_key() {
        let config = common_config();

        // try to get a non existing key
        config.get::<i32>("non exisiting key").unwrap_err();
    }

    #[test]
    fn test_incorrect_value_type() {
        let config = common_config();

        // try to get a non existing key
        config.get::<i32>("string").unwrap_err();
    }
}
