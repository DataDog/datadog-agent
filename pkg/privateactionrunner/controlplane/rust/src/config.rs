// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::path::PathBuf;
use std::time::Duration;

use crate::par_config::ParConfig;

/// Runtime configuration for par-control, merged from CLI args and datadog.yaml.
#[derive(Clone, Debug)]
pub struct Config {
    // Executor management (CLI)
    pub executor_binary: PathBuf,
    pub executor_socket: String,
    /// Forwarded to par-executor as --cfgpath.  Defaults to the same
    /// datadog.yaml par-control reads.
    pub executor_cfgpath: Option<PathBuf>,
    /// Forwarded to par-executor as --extracfgpath (-E) for each entry.
    /// In K8s this carries /etc/datadog-agent/privateactionrunner.yaml.
    pub executor_extracfg: Vec<PathBuf>,

    // Identity / OPMS (from datadog.yaml)
    pub dd_api_host: String,
    pub org_id: i64,
    pub runner_id: String,
    pub private_key_b64: String,

    // Timing (from datadog.yaml with defaults)
    pub loop_interval: Duration,
    pub heartbeat_interval: Duration,
    pub task_timeout: Duration,
    pub executor_idle_timeout: u32,
    pub executor_start_timeout: Duration,
}

impl Config {
    pub fn new(
        executor_binary: PathBuf,
        executor_socket: String,
        executor_cfgpath: Option<PathBuf>,
        executor_extracfg: Vec<PathBuf>,
        par: ParConfig,
    ) -> Self {
        Config {
            executor_binary,
            executor_socket,
            executor_cfgpath,
            executor_extracfg,
            dd_api_host: par.dd_api_host,
            org_id: par.org_id,
            runner_id: par.runner_id,
            private_key_b64: par.private_key_b64,
            loop_interval: par.loop_interval,
            heartbeat_interval: par.heartbeat_interval,
            task_timeout: par.task_timeout,
            executor_idle_timeout: par.executor_idle_timeout,
            executor_start_timeout: par.executor_start_timeout,
        }
    }
}
