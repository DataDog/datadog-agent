use serde::Serialize;

/// JSON event payload emitted by the check, one per sub task.
///
/// TODO(dsec-140): replace with the SDS result payload (protoc generated code).
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

/// TODO(dsec-140): replace with the SDS result payload task status (protoc generated code).
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
