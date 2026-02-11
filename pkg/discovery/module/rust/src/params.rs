// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use serde::Deserialize;

#[derive(Deserialize, Debug)]
pub struct Params {
    pub new_pids: Option<Vec<i32>>,
    pub heartbeat_pids: Option<Vec<i32>>,
}
