// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! par-control is the always-on, minimal control plane of the split Private
//! Action Runner. It polls the on-prem management service (OPMS) for tasks,
//! drives the on-demand Go executor's lifecycle via the process manager,
//! dispatches actions over the local control<->executor gRPC service, forwards
//! heartbeats, and publishes results back to OPMS.
//!
//! Only the control plane touches OPMS; the executor only verifies and runs a
//! single action and streams the outcome back. See
//! `.scratch/par-rss-split/prd.md` for the full design.

pub mod bootstrap;
pub mod config;
pub mod executor;
pub mod identity;
pub mod jwt;
pub mod opms;
pub mod orchestrator;
pub mod procmgr;
pub mod proto;
pub mod tls;
pub mod transport;
