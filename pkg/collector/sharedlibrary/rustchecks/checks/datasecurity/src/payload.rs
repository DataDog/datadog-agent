use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize)]
pub struct ScanConfig {
    #[serde(default)]
    #[allow(dead_code)]
    pub r#type: Option<String>,
    pub tasks: Vec<ScanTask>,
}

#[derive(Debug, Deserialize)]
pub struct PostgresConnection {
    pub host: String,
    #[serde(default = "default_postgres_port")]
    pub port: u16,
    pub username: String,
    pub password: String,
    pub dbname: String,
}

fn default_postgres_port() -> u16 {
    5432
}

#[derive(Debug, Deserialize)]
pub struct ScanTask {
    pub scanning_rules: Vec<ScanningRule>,
    pub scan_data: ScanData,
}

#[derive(Debug, Deserialize)]
pub struct ScanningRule {
    pub id: String,
    #[allow(dead_code)]
    pub name: String,
    pub regex: String,
}

#[derive(Debug, Deserialize)]
pub struct ScanData {
    pub postgres: Option<PostgresScanData>,
}

#[derive(Debug, Deserialize)]
pub struct PostgresScanData {
    pub query: String,
    pub table: String,
}

/// JSON representation of the scan result used to build
/// `proto/datadog/sds/sds_result.proto` (see `proto.rs`).
#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SdsResultPayload {
    pub scan_source: &'static str,
    pub timestamp: i64,
    pub resource: Resource,
    pub scanning_source: ScanningSource,
    pub scan_results: Vec<ScanResult>,
}

#[derive(Debug, Serialize)]
pub struct Resource {
    #[serde(rename = "type")]
    pub resource_type: String,
    pub name: String,
}

#[derive(Debug, Serialize)]
pub struct ScanningSource {
    pub agentless: Agentless,
}

#[derive(Debug, Serialize)]
pub struct Agentless {
    pub version: String,
    pub region: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ScanResult {
    pub duration: i64,
    pub table_matches: Vec<TableMatch>,
    pub location: ScanLocation,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct TableMatch {
    pub rule_id: String,
    pub column_name: String,
    pub count_matched_rows: i64,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ScanLocation {
    pub database: String,
    pub rds_table: RdsTable,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct RdsTable {
    pub instance_arn: String,
    pub database_name: String,
    pub table_name: String,
}
