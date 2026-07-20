// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

mod json_utils;
mod parse_json;
mod redact;

pub use parse_json::ParseJson;
pub use redact::Redact;

/// The curated set of VRL functions available to log processing rules. This
/// intentionally does not depend on vrl's `stdlib` feature (see the crate
/// root docs) — it's just enough for JSON-field-aware filtering and PII
/// masking, kept lean by design.
pub fn function_list() -> Vec<Box<dyn vrl::compiler::Function>> {
    vec![Box::new(ParseJson), Box::new(Redact)]
}
