use serde::Serialize;

/// JSON event payload emitted by the check, one per sub task. Kept close to the
/// shape of the real SDS result (see `sds_result.proto`) so it can be grown into
/// the full payload later. On failure, `status` is `ERROR` and `failure_reason`
/// carries the cause; on success it holds the scan `matches`.
#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ScanEventPayload {
    pub task_id: String,
    pub sub_task_id: String,
    pub status: ScanStatus,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub failure_reason: String,
    pub matches: Vec<Match>,
}

/// Outcome of a sub task scan, mirroring the proto `ScanStatus`.
#[derive(Debug, Clone, Copy, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ScanStatus {
    Success,
    Error,
}

#[derive(Debug, Serialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub struct Match {
    pub rule_id: String,
    pub column_name: String,
    pub count_matched_rows: i64,
}
