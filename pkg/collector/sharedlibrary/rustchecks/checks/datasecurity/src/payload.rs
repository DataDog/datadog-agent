/// Outcome of a sub task scan, mirroring the proto `ScanStatus`.
#[derive(Debug, Clone, Copy)]
pub enum ScanStatus {
    Success,
    Error,
}

/// Aggregated scanner match for one column, converted into a proto `TableMatch`.
#[derive(Debug, PartialEq)]
pub struct Match {
    pub rule_id: String,
    pub column_name: String,
    pub count_matched_rows: i64,
}
