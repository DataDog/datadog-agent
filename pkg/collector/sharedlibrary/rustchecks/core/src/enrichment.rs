use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Enrichment data provided by the check host during adapter initialization.
/// This struct must match the ACR's EnrichmentData exactly for YAML serialization compatibility.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct EnrichmentData {
    pub hostname: String,
    pub host_tags: HashMap<String, String>,
    pub cluster_name: Option<String>,
    pub agent_version: String,
    pub config_values: HashMap<String, serde_yaml::Value>,
    pub process_start_time: u64,
    pub k8s_connection_info: Option<K8sConnectionInfo>,
}

/// Parse enrichment data from a YAML string.
/// This function is used by the `generate_ffi!` macro so that consuming crates
/// don't need to depend on `serde_yaml` directly.
pub fn parse_enrichment(yaml_str: &str) -> Result<EnrichmentData, serde_yaml::Error> {
    serde_yaml::from_str(yaml_str)
}

/// Kubernetes connection info provided as part of enrichment data.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct K8sConnectionInfo {
    pub api_server_url: String,
    pub bearer_token: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_enrichment_data_default() {
        let data = EnrichmentData::default();
        assert_eq!(data.hostname, "");
        assert!(data.host_tags.is_empty());
        assert!(data.cluster_name.is_none());
        assert_eq!(data.agent_version, "");
        assert!(data.config_values.is_empty());
        assert_eq!(data.process_start_time, 0);
        assert!(data.k8s_connection_info.is_none());
    }

    #[test]
    fn test_enrichment_data_round_trip() {
        let mut host_tags = HashMap::new();
        host_tags.insert("env".to_string(), "prod".to_string());
        host_tags.insert("region".to_string(), "us-east-1".to_string());

        let mut config_values = HashMap::new();
        config_values.insert(
            "dd_url".to_string(),
            serde_yaml::Value::String("https://app.datadoghq.com".to_string()),
        );
        config_values.insert(
            "log_level".to_string(),
            serde_yaml::Value::String("info".to_string()),
        );

        let data = EnrichmentData {
            hostname: "myhost.example.com".to_string(),
            host_tags,
            cluster_name: Some("my-cluster".to_string()),
            agent_version: "7.50.0".to_string(),
            config_values,
            process_start_time: 1700000000,
            k8s_connection_info: Some(K8sConnectionInfo {
                api_server_url: "https://kubernetes.default.svc".to_string(),
                bearer_token: Some("my-token".to_string()),
            }),
        };

        let yaml = serde_yaml::to_string(&data).expect("serialize to YAML");
        let deserialized: EnrichmentData =
            serde_yaml::from_str(&yaml).expect("deserialize from YAML");

        assert_eq!(deserialized.hostname, "myhost.example.com");
        assert_eq!(deserialized.host_tags.get("env").unwrap(), "prod");
        assert_eq!(deserialized.host_tags.get("region").unwrap(), "us-east-1");
        assert_eq!(deserialized.cluster_name.as_deref(), Some("my-cluster"));
        assert_eq!(deserialized.agent_version, "7.50.0");
        assert_eq!(deserialized.process_start_time, 1700000000);

        let k8s = deserialized.k8s_connection_info.unwrap();
        assert_eq!(k8s.api_server_url, "https://kubernetes.default.svc");
        assert_eq!(k8s.bearer_token.as_deref(), Some("my-token"));
    }

    #[test]
    fn test_enrichment_data_without_optional_fields() {
        let data = EnrichmentData {
            hostname: "host1".to_string(),
            host_tags: HashMap::new(),
            cluster_name: None,
            agent_version: "7.50.0".to_string(),
            config_values: HashMap::new(),
            process_start_time: 0,
            k8s_connection_info: None,
        };

        let yaml = serde_yaml::to_string(&data).expect("serialize to YAML");
        let deserialized: EnrichmentData =
            serde_yaml::from_str(&yaml).expect("deserialize from YAML");

        assert_eq!(deserialized.hostname, "host1");
        assert!(deserialized.cluster_name.is_none());
        assert!(deserialized.k8s_connection_info.is_none());
    }
}
