// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use log::warn;

use crate::spawn::SpawnProfile;

use super::credential::SpawnCredential;

pub(crate) fn intended_spawn_user(process_name: &str, profile: SpawnProfile) -> String {
    match SpawnCredential::resolve(profile) {
        Ok(credential) => credential.display_name(),
        Err(e) => {
            warn!("[{process_name}] could not resolve intended spawn user: {e:#}");
            "unknown".to_string()
        }
    }
}
