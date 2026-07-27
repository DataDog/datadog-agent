//! Protobuf construction for SDS results.
//!
//! The `datadog.sds` types are prost-generated: from `OUT_DIR` under cargo, from
//! the `sds_proto` crate under Bazel.

use prost::Message;

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
pub use datadog::sds::SdsResultPayload;

/// Marshal `payload` to protobuf bytes for the `sds-result` event platform track.
// Consumed by the check in a follow-up; only the test uses it for now.
#[allow(dead_code)]
pub fn encode(payload: &SdsResultPayload) -> Vec<u8> {
    payload.encode_to_vec()
}

#[cfg(test)]
mod tests {
    use prost::Message;

    use super::datadog::sds::sds_result_payload::Resource;
    use super::*;

    /// Round-trips a minimal `SdsResultPayload` through protobuf, proving the
    /// generated `datadog.sds` bindings are wired into the datasecurity crate
    /// (under both the cargo and Bazel proto paths).
    #[test]
    fn sds_result_payload_round_trips_through_protobuf() {
        let payload = SdsResultPayload {
            timestamp: 42,
            resource: Some(Resource {
                r#type: "postgres_table".to_string(),
                name: "inst.db.public.users".to_string(),
            }),
            rule_ids: vec!["rule-1".to_string()],
            ..Default::default()
        };

        let bytes = encode(&payload);
        assert!(!bytes.is_empty(), "encoded payload should not be empty");

        let decoded = SdsResultPayload::decode(bytes.as_slice())
            .expect("payload should decode from its own protobuf bytes");
        assert_eq!(decoded, payload);
    }
}
