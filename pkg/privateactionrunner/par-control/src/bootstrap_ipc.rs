// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolves the three bootstrap-only IPC settings before the Core Agent config stream is
//! available: `cmd_port`, `auth_token_file_path`, and `ipc_cert_file_path`.
//!
//! This follows the same layering ADP uses for its own bootstrap phase
//! (`agent_data_plane_config_system::LoadedConfiguration`): a `saluki_config::ConfigurationLoader`
//! reads `datadog.yaml`, then `DD_`-prefixed environment variables on top, without adopting ADP's
//! generated schema or translator, which are specific to ADP's much larger configuration surface.
//! CLI flags remain the caller's responsibility and take precedence over everything here.

use anyhow::{Context, Result};
use datadog_agent_commons::platform::PlatformSettings;
use saluki_config::{ConfigurationLoader, GenericConfiguration};
use std::path::PathBuf;

const ENV_VAR_PREFIX: &str = "DD";

/// `cmd_port`, `auth_token_file_path`, and `ipc_cert_file_path` from `datadog.yaml` and the
/// environment, without CLI overrides.
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct Settings {
    pub cmd_port: Option<u16>,
    pub auth_token_file_path: Option<PathBuf>,
    pub ipc_cert_file_path: Option<PathBuf>,
}

/// Loads settings from `datadog.yaml` at its default location and the environment.
///
/// A missing or unreadable configuration file is not an error: it leaves these settings to fall
/// back to environment variables and platform defaults, the same as an Agent installation that
/// has not customized them.
pub fn load() -> Result<Settings> {
    from_generic(&generic_config(&PlatformSettings::get_config_file_path())?)
}

fn generic_config(yaml_path: &std::path::Path) -> Result<GenericConfiguration> {
    let loader = ConfigurationLoader::default()
        .from_yaml(yaml_path)
        .unwrap_or_default();
    let loader = loader
        .from_environment(ENV_VAR_PREFIX)
        .context("reading DD_-prefixed environment variables")?;
    Ok(loader.bootstrap_generic())
}

fn from_generic(config: &GenericConfiguration) -> Result<Settings> {
    Ok(Settings {
        cmd_port: config
            .try_get_typed("cmd_port")
            .context("invalid cmd_port in datadog.yaml or the environment")?,
        auth_token_file_path: config
            .try_get_typed("auth_token_file_path")
            .context("invalid auth_token_file_path in datadog.yaml or the environment")?,
        ipc_cert_file_path: config
            .try_get_typed("ipc_cert_file_path")
            .context("invalid ipc_cert_file_path in datadog.yaml or the environment")?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn settings_from_yaml(yaml: &str) -> Settings {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");
        std::fs::write(&path, yaml).unwrap();

        from_generic(&generic_config(&path).unwrap()).unwrap()
    }

    #[test]
    fn reads_configured_settings() {
        let settings = settings_from_yaml(
            "cmd_port: 5555\nauth_token_file_path: /etc/datadog-agent/auth_token\nipc_cert_file_path: /etc/datadog-agent/ipc_cert.pem\n",
        );

        assert_eq!(settings.cmd_port, Some(5555));
        assert_eq!(
            settings.auth_token_file_path,
            Some(PathBuf::from("/etc/datadog-agent/auth_token"))
        );
        assert_eq!(
            settings.ipc_cert_file_path,
            Some(PathBuf::from("/etc/datadog-agent/ipc_cert.pem"))
        );
    }

    #[test]
    fn ignores_unrelated_keys() {
        let settings = settings_from_yaml("api_key: abc123\nlog_level: debug\ncmd_port: 9001\n");

        assert_eq!(settings.cmd_port, Some(9001));
        assert_eq!(settings.auth_token_file_path, None);
    }

    #[test]
    fn missing_keys_yield_none() {
        assert_eq!(settings_from_yaml("api_key: abc123\n"), Settings::default());
    }

    #[test]
    fn missing_file_yields_defaults() {
        let settings = from_generic(
            &generic_config(std::path::Path::new("/nonexistent/datadog.yaml")).unwrap(),
        )
        .unwrap();
        assert_eq!(settings, Settings::default());
    }

    #[test]
    fn rejects_malformed_cmd_port() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");
        std::fs::write(&path, "cmd_port: not-a-port\n").unwrap();

        let error = from_generic(&generic_config(&path).unwrap())
            .unwrap_err()
            .to_string();
        assert!(error.contains("cmd_port"));
    }
}
