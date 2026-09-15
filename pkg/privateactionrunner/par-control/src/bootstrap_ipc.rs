// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Resolves bootstrap settings before the Core Agent config stream is available: IPC
//! connection settings and the optional sidecar-local configured PAR identity.
//!
//! This follows the same layering ADP uses for its own bootstrap phase
//! (`agent_data_plane_config_system::LoadedConfiguration`): a `saluki_config::ConfigurationLoader`
//! reads `datadog.yaml`, the optional PAR extra config, then `DD_`-prefixed environment variables,
//! without adopting ADP's generated schema or translator, which are specific to ADP's much larger
//! configuration surface.
//! CLI flags remain the caller's responsibility and take precedence over everything here.

use crate::enrollment::ConfiguredIdentity;
use anyhow::{Context, Result};
use datadog_agent_commons::platform::PlatformSettings;
use saluki_config::{ConfigurationLoader, GenericConfiguration};
use std::path::{Path, PathBuf};

const ENV_VAR_PREFIX: &str = "DD";
const EXTRA_CONFIG_PATH_ENV: &str = "DD_PRIVATE_ACTION_RUNNER_EXTRA_CONFIG_PATH";
const URN_ENV: &str = "DD_PRIVATE_ACTION_RUNNER_URN";
const PRIVATE_KEY_ENV: &str = "DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY";

/// Bootstrap settings from the local configuration and environment, without CLI overrides.
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct Settings {
    pub cmd_port: Option<u16>,
    pub auth_token_file_path: Option<PathBuf>,
    pub ipc_cert_file_path: Option<PathBuf>,
    pub configured_identity: Option<ConfiguredIdentity>,
}

/// Loads settings from `datadog.yaml` at its default location and the environment.
///
/// A missing or unreadable configuration file is not an error: it leaves these settings to fall
/// back to environment variables and platform defaults, the same as an Agent installation that
/// has not customized them.
pub fn load() -> Result<Settings> {
    let extra_path = std::env::var_os(EXTRA_CONFIG_PATH_ENV).map(PathBuf::from);
    from_generic(
        &generic_config(
            &PlatformSettings::get_config_file_path(),
            extra_path.as_deref(),
        )?,
        std::env::var(URN_ENV).ok(),
        std::env::var(PRIVATE_KEY_ENV).ok(),
    )
}

fn generic_config(yaml_path: &Path, extra_path: Option<&Path>) -> Result<GenericConfiguration> {
    let mut loader = ConfigurationLoader::default()
        .from_yaml(yaml_path)
        .unwrap_or_default();
    if let Some(path) = extra_path {
        loader = loader
            .from_yaml(path)
            .context("reading the PAR extra configuration file")?;
    }
    let loader = loader
        .from_environment(ENV_VAR_PREFIX)
        .context("reading DD_-prefixed environment variables")?;
    Ok(loader.bootstrap_generic())
}

fn from_generic(
    config: &GenericConfiguration,
    environment_urn: Option<String>,
    environment_private_key: Option<String>,
) -> Result<Settings> {
    let urn = match environment_urn {
        Some(urn) => Some(urn),
        None => config
            .try_get_typed("private_action_runner.urn")
            .context("invalid private_action_runner.urn")?,
    };
    let private_key = match environment_private_key {
        Some(private_key) => Some(private_key),
        None => config
            .try_get_typed("private_action_runner.private_key")
            .context("invalid private_action_runner.private_key")?,
    };
    let configured_identity = match (urn, private_key) {
        (None, None) => None,
        (urn, private_key) => Some(ConfiguredIdentity {
            urn: urn.unwrap_or_default(),
            private_key: private_key.unwrap_or_default(),
        }),
    };

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
        configured_identity,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn settings_from_yaml(yaml: &str) -> Settings {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");
        std::fs::write(&path, yaml).unwrap();

        from_generic(&generic_config(&path, None).unwrap(), None, None).unwrap()
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
            &generic_config(std::path::Path::new("/nonexistent/datadog.yaml"), None).unwrap(),
            None,
            None,
        )
        .unwrap();
        assert_eq!(settings, Settings::default());
    }

    #[test]
    fn rejects_malformed_cmd_port() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");
        std::fs::write(&path, "cmd_port: not-a-port\n").unwrap();

        let error = from_generic(&generic_config(&path, None).unwrap(), None, None)
            .unwrap_err()
            .to_string();
        assert!(error.contains("cmd_port"));
    }

    #[test]
    fn extra_config_supplies_identity() {
        let dir = tempfile::tempdir().unwrap();
        let main = dir.path().join("datadog.yaml");
        let extra = dir.path().join("privateactionrunner.yaml");
        std::fs::write(
            &main,
            "private_action_runner:\n  urn: main\n  private_key: main-key\n",
        )
        .unwrap();
        std::fs::write(
            &extra,
            "private_action_runner:\n  urn: extra\n  private_key: extra-key\n",
        )
        .unwrap();

        let settings =
            from_generic(&generic_config(&main, Some(&extra)).unwrap(), None, None).unwrap();
        let identity = settings.configured_identity.unwrap();
        assert_eq!(identity.urn, "extra");
        assert_eq!(identity.private_key, "extra-key");
    }

    #[test]
    fn environment_identity_takes_precedence() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("datadog.yaml");
        std::fs::write(
            &path,
            "private_action_runner:\n  urn: file\n  private_key: file-key\n",
        )
        .unwrap();
        let config = generic_config(&path, None).unwrap();

        let settings = from_generic(
            &config,
            Some("environment".to_string()),
            Some("environment-key".to_string()),
        )
        .unwrap();
        let identity = settings.configured_identity.unwrap();
        assert_eq!(identity.urn, "environment");
        assert_eq!(identity.private_key, "environment-key");
    }

    #[test]
    fn preserves_partial_identity_for_endpoint_validation() {
        let config = ConfigurationLoader::default().bootstrap_generic();
        let settings = from_generic(&config, Some("urn-only".to_string()), None).unwrap();
        let identity = settings.configured_identity.unwrap();
        assert_eq!(identity.urn, "urn-only");
        assert!(identity.private_key.is_empty());
    }
}
