//! Agent metadata constants for SDS result payloads.

pub const RESOURCE_TYPE_RDS_INSTANCE: &str = "aws_rds_instance";
pub const AGENTLESS_VERSION: &str = "1.0.0";
pub const AGENTLESS_REGION: &str = "us-east-1";

/// Event platform track type for SDS results (matches Go `EventTypeSDSResult`).
pub const SDS_RESULT_EVENT_TYPE: &str = "sds-result";
