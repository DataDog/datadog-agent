// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::ProcessConfig;
use tokio::sync::oneshot;
use tonic::Status;

pub struct ReloadResult {
    pub added: Vec<String>,
    pub removed: Vec<String>,
    pub modified: Vec<String>,
    pub unchanged: Vec<String>,
}

pub enum Command {
    Create {
        name: String,
        config: Box<ProcessConfig>,
        reply: oneshot::Sender<Result<(), Status>>,
    },
    Start {
        name: String,
        reply: oneshot::Sender<Result<(), Status>>,
    },
    Stop {
        name: String,
        reply: oneshot::Sender<Result<(), Status>>,
    },
    ReloadConfig {
        reply: oneshot::Sender<Result<ReloadResult, Status>>,
    },
}
