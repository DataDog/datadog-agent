// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::info;

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest};

use super::super::JobObject;
use super::credential::SpawnCredential;
use super::inherit_supervisor::spawn_inherit_supervisor;
use super::primary_token::spawn_as_primary_token;
use super::privileged;

impl ManagedProcess {
    pub(crate) fn spawn_child_handle(&mut self) -> Result<ProcessHandle> {
        let profile = self.profile();
        info!("[{}] spawn profile: {profile}", self.name());

        match profile {
            SpawnProfile::Privileged => self.spawn_privileged(),
            SpawnProfile::Agent => self.spawn_agent(),
        }
    }

    fn spawn_request(&self) -> Result<SpawnRequest> {
        SpawnRequest::from_config(self.name(), self.config())
    }

    fn spawn_agent(&mut self) -> Result<ProcessHandle> {
        let credential = match self.agent_credential().cloned() {
            Some(credential) => credential,
            None => {
                let credential = SpawnCredential::resolve_agent()
                    .with_context(|| format!("[{}] resolve spawn credential", self.name()))?;
                self.set_intended_user(credential.display_name());
                credential
            }
        };

        if credential.reuses_supervisor_token() {
            return self.spawn_with_supervisor_inherit(&credential);
        }

        self.spawn_agent_logon(&credential)
    }

    fn spawn_agent_logon(&mut self, credential: &SpawnCredential) -> Result<ProcessHandle> {
        let request = self.spawn_request()?;
        let job = JobObject::new().with_context(|| {
            format!("[{}] create job object for child supervision", self.name())
        })?;

        let (handle, user_profile) =
            spawn_as_primary_token(self.name(), &request, credential, &job)
                .with_context(|| format!("[{}] CreateProcessAsUserW spawn failed", self.name()))?;

        self.set_user_profile_guard(user_profile);

        self.set_job_object(job);
        Ok(handle)
    }

    fn spawn_privileged(&mut self) -> Result<ProcessHandle> {
        let request = self.spawn_request()?;
        privileged::validate_process_request(self.name(), &request)?;
        privileged::validate_supervisor_is_local_system(self.name())?;
        self.spawn_with_supervisor_inherit(&SpawnCredential::privileged())
    }

    fn spawn_with_supervisor_inherit(
        &mut self,
        credential: &SpawnCredential,
    ) -> Result<ProcessHandle> {
        let request = self.spawn_request()?;
        let job = JobObject::new().with_context(|| {
            format!("[{}] create job object for child supervision", self.name())
        })?;

        let handle = spawn_inherit_supervisor(self.name(), &request, credential, &job)
            .with_context(|| format!("[{}] supervisor-token inherit spawn failed", self.name()))?;

        self.set_job_object(job);
        Ok(handle)
    }
}
