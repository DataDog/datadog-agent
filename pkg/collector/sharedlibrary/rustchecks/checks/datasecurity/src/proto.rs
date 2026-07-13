//! Wire-format encoding for `SdsResultPayload` using the canonical agent proto.
//!
//! Cargo/IDE builds compile `pkg/proto/datadog/sds/sds_result.proto` via
//! `build.rs` (prost-build) and include the generated code from `OUT_DIR`.
//! Bazel builds instead depend on the `sds_proto` crate produced by
//! `//pkg/proto/datadog/sds:sds_rust_proto` (rules_rust_prost), mirroring the
//! `pkg/procmgr/rust` pattern.

use prost::Message;

use crate::payload::SdsResultPayload as Payload;

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

use datadog::sds::{
    scanning_source::{Agentless, Source},
    sds_result_payload::{
        scan_location::ScanLocation as ScanLocationKind, Resource, RdsTable, ScanResult,
        ScanSource, ScanLocation as ProtoScanLocation, TableMatch,
    },
    ScanningSource, SdsResultPayload,
};

/// Convert the in-memory scan result into the protobuf message defined in
/// `pkg/proto/datadog/sds/sds_result.proto`.
pub fn to_proto(payload: &Payload) -> SdsResultPayload {
    SdsResultPayload {
        scan_source: ScanSource::Agentless as i32,
        timestamp: payload.timestamp,
        resource: Some(Resource {
            r#type: payload.resource.resource_type.clone(),
            name: payload.resource.name.clone(),
        }),
        scan_results: payload
            .scan_results
            .iter()
            .map(|result| ScanResult {
                matches: vec![],
                location: Some(ProtoScanLocation {
                    scan_location_type: None,
                    size_in_bytes: 0,
                    last_modified_timestamp: 0,
                    table: String::new(),
                    scan_location: Some(ScanLocationKind::RdsTable(RdsTable {
                        instance_arn: result.location.rds_table.instance_arn.clone(),
                        snapshot_arn: String::new(),
                        snapshot_timestamp: 0,
                        database_name: result.location.rds_table.database_name.clone(),
                        table_name: result.location.rds_table.table_name.clone(),
                        table_row_count: None,
                        scanned_row_count: None,
                    })),
                }),
                duration: result.duration,
                table_matches: result
                    .table_matches
                    .iter()
                    .map(|m| TableMatch {
                        rule_id: m.rule_id.clone(),
                        column_name: m.column_name.clone(),
                        count_matched_rows: m.count_matched_rows,
                        count_total_rows: 0,
                    })
                    .collect(),
            })
            .collect(),
        scan_stats: None,
        scanner_metadata: None,
        rules: Default::default(),
        rule_ids: Default::default(),
        scanning_source: Some(ScanningSource {
            source: Some(Source::Agentless(Agentless {
                version: payload.scanning_source.agentless.version.clone(),
                region: payload.scanning_source.agentless.region.clone(),
            })),
        }),
    }
}

/// Marshal `payload` to protobuf bytes for the `sds-result` event platform track.
pub fn encode(payload: &Payload) -> Vec<u8> {
    to_proto(payload).encode_to_vec()
}
