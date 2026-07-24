//! Protobuf construction for SDS results.
//!
//! The `datadog.sds` types are prost-generated: from `OUT_DIR` under cargo, from
//! the `sds_proto` crate under Bazel.

use std::time::{SystemTime, UNIX_EPOCH};

use prost::Message;

use crate::payload::Match;

// cargo maps well-known types to `prost-types`; the Bazel toolchain to a `timestamp_proto` crate.
#[cfg(not(bazel))]
use prost_types::Timestamp;
#[cfg(bazel)]
use timestamp_proto::google::protobuf::Timestamp;

#[cfg(not(bazel))]
pub mod datadog {
    pub mod sds {
        include!(concat!(env!("OUT_DIR"), "/datadog.sds.rs"));
    }
}

#[cfg(bazel)]
pub mod datadog {
    pub mod sds {
        pub use sds_proto::datadog::sds::*;
    }
}

// Proto types the check assembles directly.
pub use datadog::sds::{
    ScanningSource, SdsResultPayload, scanning_source,
    sds_result_payload::{
        PostgresTable, Resource, ScanLocation, ScanMetadata, ScanResult, TableMatch, scan_location,
        scan_metadata::{ScanTaskMetadata, scan_task_metadata::Status},
    },
};

/// Convert aggregated scanner matches into proto `TableMatch` entries.
pub fn table_matches(matches: &[Match]) -> Vec<TableMatch> {
    matches
        .iter()
        .map(|m| TableMatch {
            rule_id: m.rule_id.clone(),
            column_name: m.column_name.clone(),
            count_matched_rows: m.count_matched_rows,
            ..Default::default()
        })
        .collect()
}

/// Convert a wall-clock time into a proto `Timestamp`.
pub fn to_timestamp(time: SystemTime) -> Timestamp {
    let (seconds, nanos) = match time.duration_since(UNIX_EPOCH) {
        Ok(d) => (d.as_secs() as i64, d.subsec_nanos() as i32),
        Err(e) => {
            let d = e.duration();
            (-(d.as_secs() as i64), -(d.subsec_nanos() as i32))
        }
    };
    Timestamp { seconds, nanos }
}

/// Marshal `payload` to protobuf bytes for the `sds-result` event platform track.
pub fn encode(payload: &SdsResultPayload) -> Vec<u8> {
    payload.encode_to_vec()
}
