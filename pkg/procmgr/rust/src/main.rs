// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use log::info;
use tokio::signal::unix::{SignalKind, signal};

#[tokio::main]
async fn main() -> Result<()> {
    simple_logger::init_with_level(log::Level::Info)?;
    info!(
        "dd-procmgrd starting (version {})",
        env!("CARGO_PKG_VERSION")
    );

    let mut sigterm = signal(SignalKind::terminate())?;
    let mut sigint = signal(SignalKind::interrupt())?;

    tokio::select! {
        _ = sigterm.recv() => info!("received SIGTERM"),
        _ = sigint.recv() => info!("received SIGINT"),
    }

    info!("dd-procmgrd shutting down");
    Ok(())
}
