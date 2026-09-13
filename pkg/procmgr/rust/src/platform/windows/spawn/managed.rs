// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::{info, warn};

use crate::handle::ProcessHandle;
use crate::process::ManagedProcess;
use crate::spawn::{SpawnProfile, SpawnRequest};

use super::super::JobObject;
use super::credential::SpawnCredential;
use super::inherit_supervisor::spawn_inherit_supervisor;
use super::primary_token::spawn_as_primary_token;

const PRIVILEGED_INTENDED_USER: &str = r"NT AUTHORITY\SYSTEM";

pub(crate) fn resolve_spawn_identity(
    process_name: &str,
    profile: SpawnProfile,
) -> (String, Option<SpawnCredential>) {
    match profile {
        SpawnProfile::Privileged => (PRIVILEGED_INTENDED_USER.to_string(), None),
        SpawnProfile::Agent => match SpawnCredential::resolve_agent() {
            Ok(credential) => (credential.display_name(), Some(credential)),
            Err(e) => {
                warn!("[{process_name}] could not resolve intended spawn user: {e:#}");
                ("unknown".to_string(), None)
            }
        },
    }
}

impl ManagedProcess {
    pub(crate) fn spawn_child_handle(&mut self) -> Result<ProcessHandle> {
        let profile = self.profile();
        let request = SpawnRequest::from_config(self.name(), self.config())?;

        info!("[{}] spawn profile: {profile}", self.name());

        if matches!(profile, SpawnProfile::Privileged) {
            return self.spawn_privileged(request);
        }

        self.spawn_agent(request)
    }

    fn spawn_agent(&mut self, request: SpawnRequest) -> Result<ProcessHandle> {
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
            return self.spawn_with_supervisor_inherit(request, &credential);
        }

        self.spawn_agent_logon(request, &credential)
    }

    fn spawn_agent_logon(
        &mut self,
        request: SpawnRequest,
        credential: &SpawnCredential,
    ) -> Result<ProcessHandle> {
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

    fn spawn_privileged(&mut self, request: SpawnRequest) -> Result<ProcessHandle> {
        self.spawn_with_supervisor_inherit(request, &SpawnCredential::privileged())
    }

    fn spawn_with_supervisor_inherit(
        &mut self,
        request: SpawnRequest,
        credential: &SpawnCredential,
    ) -> Result<ProcessHandle> {
        let job = JobObject::new().with_context(|| {
            format!("[{}] create job object for child supervision", self.name())
        })?;

        let handle = spawn_inherit_supervisor(self.name(), &request, credential, &job)
            .with_context(|| format!("[{}] supervisor-token inherit spawn failed", self.name()))?;

        self.set_job_object(job);
        Ok(handle)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::spawn::SpawnProfile;

    #[test]
    fn privileged_profile_spawn_user_is_local_system() {
        assert_eq!(
            resolve_spawn_identity("datadog-agent-process", SpawnProfile::Privileged).0,
            PRIVILEGED_INTENDED_USER
        );
    }
}
