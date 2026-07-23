use serde::Serialize;

/// JSON event payload emitted by the check. Kept close to the shape of the real
/// SDS result so it can be grown into the full payload later.
#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ScanEventPayload {
    pub task_id: String,
    pub sub_task_id: String,
    pub matches: Vec<Match>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct Match {
    pub rule_id: String,
    pub column_name: String,
    pub count_matched_rows: i64,
}
