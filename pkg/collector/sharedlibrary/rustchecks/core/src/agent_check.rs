use anyhow::Result;

use super::aggregator::{CallbackContext, LogLevel, MetricType, ServiceCheckStatus};
use super::config::Config;
use super::enrichment::{EnrichmentData, K8sConnectionInfo};

use std::collections::HashMap;
use std::ffi::c_int;
use std::os::raw::c_ulong;

pub struct AgentCheck {
    callback: CallbackContext,  // submit callbacks via ACR-compatible ABI
    enrichment: EnrichmentData, // enrichment data from the host

    // configuration fields are made public to mimic their uses in Python checks
    pub init_config: Config, // common configuration for each instance
    pub instance: Config,    // instance specific configuration
}

impl AgentCheck {
    pub fn new(
        init_config: Config,
        instance_config: Config,
        callback: CallbackContext,
        enrichment: EnrichmentData,
    ) -> Self {
        Self {
            callback,
            enrichment,
            init_config,
            instance: instance_config,
        }
    }

    /// Get the hostname from enrichment data
    pub fn get_hostname(&self) -> &str {
        &self.enrichment.hostname
    }

    /// Get host tags from enrichment data
    pub fn get_host_tags(&self) -> &HashMap<String, String> {
        &self.enrichment.host_tags
    }

    /// Get the cluster name from enrichment data
    pub fn get_clustername(&self) -> Option<&str> {
        self.enrichment.cluster_name.as_deref()
    }

    /// Get the agent version from enrichment data
    pub fn get_version(&self) -> &str {
        &self.enrichment.agent_version
    }

    /// Get a config value from enrichment data
    pub fn get_config(&self, key: &str) -> Option<&serde_yaml::Value> {
        self.enrichment.config_values.get(key)
    }

    /// Get the process start time from enrichment data
    pub fn get_process_start_time(&self) -> u64 {
        self.enrichment.process_start_time
    }

    /// Get the Kubernetes connection info from enrichment data
    pub fn get_connection_info(&self) -> Option<&K8sConnectionInfo> {
        self.enrichment.k8s_connection_info.as_ref()
    }

    /// Submit a log message via the callback
    pub fn log(&self, level: LogLevel, message: &str) -> Result<()> {
        self.callback.submit_log(level, message)
    }

    /// Send Gauge metric
    pub fn gauge(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Gauge,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Rate metric
    pub fn rate(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Rate,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Count metric
    pub fn count(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Count,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Monotonic Count metric
    pub fn monotonic_count(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::MonotonicCount,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Decrement metric
    pub fn decrement(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Counter,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Histogram metric
    pub fn histogram(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Histogram,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Historate metric
    pub fn historate(
        &self,
        name: &str,
        value: f64,
        tags: &[String],
        hostname: &str,
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_metric(
            MetricType::Historate,
            name,
            value,
            tags,
            hostname,
            flush_first_value,
        )
    }

    /// Send Service Check
    pub fn service_check(
        &self,
        name: &str,
        status: ServiceCheckStatus,
        tags: &[String],
        hostname: &str,
        message: &str,
    ) -> Result<()> {
        self.callback
            .submit_service_check(name, status, tags, hostname, message)
    }

    /// Send Event
    pub fn event(
        &self,
        title: &str,
        text: &str,
        timestamp: c_ulong,
        priority: &str,
        host: &str,
        tags: &[String],
        alert_type: &str,
        aggregation_key: &str,
        source_type_name: &str,
        event_type: &str,
    ) -> Result<()> {
        self.callback.submit_event(
            title,
            text,
            timestamp,
            priority,
            host,
            tags,
            alert_type,
            aggregation_key,
            source_type_name,
            event_type,
        )
    }

    /// Send Histogram Bucket
    pub fn submit_histogram_bucket(
        &self,
        metric_name: &str,
        value: i64,
        lower_bound: f32,
        upper_bound: f32,
        monotonic: c_int,
        hostname: &str,
        tags: &[String],
        flush_first_value: bool,
    ) -> Result<()> {
        self.callback.submit_histogram_bucket(
            metric_name,
            value,
            lower_bound,
            upper_bound,
            monotonic,
            hostname,
            tags,
            flush_first_value,
        )
    }

    /// Send Event Platform Event
    pub fn event_platform_event(&self, raw_event: &str, event_track_type: &str) -> Result<()> {
        self.callback.submit_event_platform_event(
            raw_event,
            raw_event.len() as c_int,
            event_track_type,
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Helper to create a mock CallbackContext for testing.
    // All callback functions are no-ops.
    unsafe extern "C" fn mock_submit_metric(
        _ctx: *mut std::ffi::c_void,
        _metric_type: c_int,
        _name: *const std::ffi::c_char,
        _value: std::ffi::c_double,
        _tags: *const *const std::ffi::c_char,
        _hostname: *const std::ffi::c_char,
        _flush_first: c_int,
    ) {
    }

    unsafe extern "C" fn mock_submit_service_check(
        _ctx: *mut std::ffi::c_void,
        _name: *const std::ffi::c_char,
        _status: c_int,
        _tags: *const *const std::ffi::c_char,
        _hostname: *const std::ffi::c_char,
        _message: *const std::ffi::c_char,
    ) {
    }

    unsafe extern "C" fn mock_submit_event(
        _ctx: *mut std::ffi::c_void,
        _event: *const super::super::aggregator::Event,
    ) {
    }

    unsafe extern "C" fn mock_submit_histogram(
        _ctx: *mut std::ffi::c_void,
        _name: *const std::ffi::c_char,
        _value: std::ffi::c_longlong,
        _lower: std::ffi::c_float,
        _upper: std::ffi::c_float,
        _monotonic: c_int,
        _hostname: *const std::ffi::c_char,
        _tags: *const *const std::ffi::c_char,
        _flush_first: c_int,
    ) {
    }

    unsafe extern "C" fn mock_submit_event_platform_event(
        _ctx: *mut std::ffi::c_void,
        _event: *const std::ffi::c_char,
        _event_len: c_int,
        _event_type: *const std::ffi::c_char,
    ) {
    }

    unsafe extern "C" fn mock_submit_log(
        _ctx: *mut std::ffi::c_void,
        _level: c_int,
        _message: *const std::ffi::c_char,
    ) {
    }

    fn mock_callback() -> super::super::aggregator::Callback {
        super::super::aggregator::Callback {
            submit_metric: mock_submit_metric,
            submit_service_check: mock_submit_service_check,
            submit_event: mock_submit_event,
            submit_histogram: mock_submit_histogram,
            submit_event_platform_event: mock_submit_event_platform_event,
            submit_log: mock_submit_log,
        }
    }

    fn mock_callback_context() -> CallbackContext {
        let cb = mock_callback();
        unsafe { CallbackContext::from_ptr(&cb as *const _, std::ptr::null_mut()) }
    }

    #[test]
    fn test_agent_check_with_default_enrichment() {
        let init_config = Config::from_str("{}").unwrap();
        let instance_config = Config::from_str("{}").unwrap();
        let callback = mock_callback_context();
        let enrichment = EnrichmentData::default();

        let check = AgentCheck::new(init_config, instance_config, callback, enrichment);

        assert_eq!(check.get_hostname(), "");
        assert!(check.get_host_tags().is_empty());
        assert!(check.get_clustername().is_none());
        assert_eq!(check.get_version(), "");
        assert!(check.get_config("nonexistent").is_none());
        assert_eq!(check.get_process_start_time(), 0);
        assert!(check.get_connection_info().is_none());
    }

    #[test]
    fn test_agent_check_with_enrichment() {
        let init_config = Config::from_str("{}").unwrap();
        let instance_config = Config::from_str("{}").unwrap();
        let callback = mock_callback_context();

        let mut host_tags = HashMap::new();
        host_tags.insert("env".to_string(), "staging".to_string());

        let mut config_values = HashMap::new();
        config_values.insert(
            "dd_url".to_string(),
            serde_yaml::Value::String("https://app.datadoghq.com".to_string()),
        );

        let enrichment = EnrichmentData {
            hostname: "myhost".to_string(),
            host_tags,
            cluster_name: Some("k8s-cluster".to_string()),
            agent_version: "7.50.0".to_string(),
            config_values,
            process_start_time: 1700000000,
            k8s_connection_info: Some(K8sConnectionInfo {
                api_server_url: "https://k8s.local:6443".to_string(),
                bearer_token: Some("token123".to_string()),
            }),
        };

        let check = AgentCheck::new(init_config, instance_config, callback, enrichment);

        assert_eq!(check.get_hostname(), "myhost");
        assert_eq!(check.get_host_tags().get("env").unwrap(), "staging");
        assert_eq!(check.get_clustername(), Some("k8s-cluster"));
        assert_eq!(check.get_version(), "7.50.0");
        assert_eq!(check.get_process_start_time(), 1700000000);

        let config_val = check.get_config("dd_url").unwrap();
        assert_eq!(
            config_val,
            &serde_yaml::Value::String("https://app.datadoghq.com".to_string())
        );
        assert!(check.get_config("missing_key").is_none());

        let k8s = check.get_connection_info().unwrap();
        assert_eq!(k8s.api_server_url, "https://k8s.local:6443");
        assert_eq!(k8s.bearer_token.as_deref(), Some("token123"));
    }
}
