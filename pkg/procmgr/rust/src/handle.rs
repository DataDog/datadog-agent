// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;
use std::process::ExitStatus;
use tokio::process::Child;

pub(crate) struct ProcessHandle {
    child: Child,
}

impl ProcessHandle {
    pub(crate) fn from_child(child: Child) -> Self {
        Self { child }
    }

    pub(crate) fn id(&self) -> Option<u32> {
        self.child.id()
    }

    pub(crate) async fn wait(&mut self) -> Result<ExitStatus> {
        Ok(self.child.wait().await?)
    }

    pub(crate) async fn kill(&mut self) -> Result<()> {
        self.child.kill().await?;
        Ok(())
    }
}
