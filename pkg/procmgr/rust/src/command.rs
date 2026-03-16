// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::ProcessConfig;
use crate::state::ProcessState;
use tokio::sync::oneshot;
use tonic::Status;

pub struct ReloadResult {
    pub added: Vec<String>,
    pub removed: Vec<String>,
    pub modified: Vec<String>,
    pub unchanged: Vec<String>,
}

#[derive(Debug)]
pub struct CreateResult {
    pub uuid: String,
}

#[derive(Debug)]
pub struct StartResult {
    pub uuid: String,
    pub pid: Option<u32>,
    pub state: ProcessState,
}

#[derive(Debug)]
pub struct StopResult {
    pub uuid: String,
    pub state: ProcessState,
}

pub enum Command {
    Create {
        name: String,
        config: Box<ProcessConfig>,
        reply: oneshot::Sender<Result<CreateResult, Status>>,
    },
    Start {
        name_or_uuid: String,
        reply: oneshot::Sender<Result<StartResult, Status>>,
    },
    Stop {
        name_or_uuid: String,
        reply: oneshot::Sender<Result<StopResult, Status>>,
    },
    ReloadConfig {
        reply: oneshot::Sender<Result<ReloadResult, Status>>,
    },
}
