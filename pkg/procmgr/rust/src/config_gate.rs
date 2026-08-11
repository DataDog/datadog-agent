// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Stub config gates for stacked PR 1. Full implementation lands in PR 2.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Deserialize, Serialize)]
pub struct ConditionConfigFile {
    pub path: String,
    #[serde(default)]
    pub keys: Vec<String>,
}

pub fn condition_config_any_met(_conditions: &[ConditionConfigFile]) -> bool {
    true
}

pub fn condition_config_summary(_conditions: &[ConditionConfigFile]) -> String {
    String::new()
}

pub fn clear_secret_caches() {}
