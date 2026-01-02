use std::collections::HashMap;
use std::option::Option;
use std::path::PathBuf;
use std::time::Duration;
use std::vec::Vec;

use http::{HeaderMap, Method, Uri};
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize, Default, Clone, Debug)]
#[serde(deny_unknown_fields)]
pub struct Global {
    pub init_config: Init,
    pub instances: Vec<Instance>,
}

#[derive(Serialize, Deserialize, Default, Clone, Debug)]
#[serde(deny_unknown_fields)]
pub struct Init {
    pub timeout: Option<u64>,
}

#[derive(Serialize, Deserialize, Clone, Default, Debug)]
#[serde(deny_unknown_fields)]
pub struct Instance {
    pub name: String,
    #[serde(with = "http_serde::uri")]
    pub url: Uri,

    #[serde(with = "http_serde::option::method")]
    pub method: Option<Method>,
    pub data: Option<String>,

    pub content_match: Option<String>,
    pub reverse_content_match: Option<bool>,

    pub http_response_status_code: Option<String>,
    pub include_content: Option<bool>,
    pub collect_response_time: Option<bool>,

    pub check_certificate_expiration: Option<bool>,
    pub days_warning: Option<u64>,
    pub days_critical: Option<u64>,
    pub seconds_warning: Option<u64>,
    pub seconds_critical: Option<u64>,

    pub tags: Option<HashMap<String, String>>,

    pub tls_verify: Option<bool>,
    pub tls_ignore_warning: Option<bool>,
    pub tls_cert: Option<PathBuf>,

    #[serde(with = "http_serde::option::header_map")]
    pub headers: Option<HeaderMap>,

    pub timeout: Option<u64>,
    pub connect_timeout: Option<u64>,
    pub read_timeout: Option<u64>,
}

pub mod defaults {
    use super::*;

    pub const METHOD: Method = Method::GET;
    pub const REVERSE_CONTENT_MATCH: bool = false;
    pub const HTTP_RESPONSE_STATUS_CODE: &str = r"(1|2|3)\d\d";

    pub const INCLUDE_CONTENT: bool = false;
    pub const COLLECT_RESPONSE_TIME: bool = true;

    pub const CHECK_CERTIFICATE_EXPIRATION: bool = true;
    pub const DAYS_WARNING: u64 = 14;
    pub const DAYS_CRITICAL: u64 = 7;

    pub const TIMEOUT: Duration = Duration::from_secs(10);

    pub const TLS_VERIFY: bool = false;
}
