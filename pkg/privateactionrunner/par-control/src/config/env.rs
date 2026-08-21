// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result, anyhow, bail};
use figment::providers::Serialized;
use figment::value::{Dict, Map};
use figment::{Error, Metadata, Profile, Provider};
use serde_json::Value;

#[derive(Clone, Copy)]
enum Decode {
    String,
    Bool,
    Integer,
    StringMap,
    StringList,
}

#[derive(Clone, Copy)]
struct EnvKey {
    aliases: &'static [&'static str],
    path: &'static str,
    decode: Decode,
    ignore_empty: bool,
}

const PAR_CONTROL_KEYS: &[EnvKey] = &[
    EnvKey::string(&["DD_SITE"], "site"),
    EnvKey::nonempty_string(&["DD_LOG_LEVEL"], "log_level"),
    EnvKey::string(&["DD_DD_URL", "DD_URL"], "dd_url"),
    EnvKey::string(&["DD_FLEET_POLICIES_DIR"], "fleet_policies_dir"),
    EnvKey::string(&["DD_IPC_CERT_FILE_PATH"], "ipc_cert_file_path"),
    EnvKey::string(&["DD_AUTH_TOKEN_FILE_PATH"], "auth_token_file_path"),
    EnvKey::boolean(&["DD_NO_PROXY_NONEXACT_MATCH"], "no_proxy_nonexact_match"),
    EnvKey::boolean(&["DD_SKIP_SSL_VALIDATION"], "skip_ssl_validation"),
    EnvKey::string(&["DD_MIN_TLS_VERSION"], "min_tls_version"),
    EnvKey::string(&["DD_PROXY_HTTP", "HTTP_PROXY", "http_proxy"], "proxy.http"),
    EnvKey::string(
        &["DD_PROXY_HTTPS", "HTTPS_PROXY", "https_proxy"],
        "proxy.https",
    ),
    EnvKey::string_list(
        &["DD_PROXY_NO_PROXY", "NO_PROXY", "no_proxy"],
        "proxy.no_proxy",
    ),
    EnvKey::boolean(
        &["DD_PRIVATE_ACTION_RUNNER_ENABLED"],
        "private_action_runner.enabled",
    ),
    EnvKey::boolean(
        &["DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"],
        "private_action_runner.split_enabled",
    ),
    EnvKey::boolean(
        &["DD_PRIVATE_ACTION_RUNNER_SELF_ENROLL"],
        "private_action_runner.self_enroll",
    ),
    EnvKey::string(
        &["DD_PRIVATE_ACTION_RUNNER_URN"],
        "private_action_runner.urn",
    ),
    EnvKey::string(
        &["DD_PRIVATE_ACTION_RUNNER_PRIVATE_KEY"],
        "private_action_runner.private_key",
    ),
    EnvKey::string(
        &["DD_PRIVATE_ACTION_RUNNER_IDENTITY_FILE_PATH"],
        "private_action_runner.identity_file_path",
    ),
    EnvKey::integer(
        &["DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY"],
        "private_action_runner.task_concurrency",
    ),
    EnvKey::string_map(
        &["DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS"],
        "private_action_runner.opms_extra_headers",
    ),
    EnvKey::string(
        &["DD_PRIVATE_ACTION_RUNNER_EXECUTOR_SOCKET_PATH"],
        "private_action_runner.executor.socket_path",
    ),
];

impl EnvKey {
    const fn string(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::String,
            ignore_empty: false,
        }
    }

    const fn nonempty_string(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::String,
            ignore_empty: true,
        }
    }

    const fn boolean(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::Bool,
            ignore_empty: true,
        }
    }

    const fn integer(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::Integer,
            ignore_empty: false,
        }
    }

    const fn string_map(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::StringMap,
            ignore_empty: false,
        }
    }

    const fn string_list(aliases: &'static [&'static str], path: &'static str) -> Self {
        Self {
            aliases,
            path,
            decode: Decode::StringList,
            ignore_empty: false,
        }
    }
}

/// Environment values that participate in the normal configuration precedence chain.
pub(super) struct ParControlEnvProvider {
    values: Value,
}

impl ParControlEnvProvider {
    pub(super) fn new(env: &impl Fn(&str) -> Option<String>) -> Result<Self> {
        Ok(Self {
            values: read_keys(PAR_CONTROL_KEYS, env)?,
        })
    }
}

fn read_keys(keys: &[EnvKey], env: &impl Fn(&str) -> Option<String>) -> Result<Value> {
    let mut values = Value::Object(serde_json::Map::new());
    for key in keys {
        let Some((name, raw)) = key
            .aliases
            .iter()
            .find_map(|name| env(name).map(|value| (*name, value)))
        else {
            continue;
        };
        if key.ignore_empty && raw.is_empty() {
            continue;
        }
        let value = decode(name, &raw, key.decode)?;
        upsert(&mut values, key.path, value);
    }
    Ok(values)
}

fn decode(name: &str, raw: &str, decode: Decode) -> Result<Value> {
    match decode {
        Decode::String => Ok(Value::String(raw.to_string())),
        Decode::Bool => match raw.trim() {
            "1" | "t" | "T" | "TRUE" | "true" | "True" => Ok(Value::Bool(true)),
            "0" | "f" | "F" | "FALSE" | "false" | "False" => Ok(Value::Bool(false)),
            _ => bail!("invalid boolean value for {name}: {raw:?}"),
        },
        Decode::Integer => raw
            .trim()
            .parse::<u64>()
            .map(|value| Value::Number(value.into()))
            .map_err(|error| anyhow!("invalid value for {name}: {error}")),
        Decode::StringMap => {
            let values: std::collections::HashMap<String, String> = serde_json::from_str(raw)
                .with_context(|| format!("{name} must be a JSON object of string values"))?;
            serde_json::to_value(values).context("failed to serialize environment map")
        }
        Decode::StringList => Ok(Value::Array(
            raw.split(|c: char| c == ',' || c.is_ascii_whitespace())
                .filter(|entry| !entry.is_empty())
                .map(|entry| Value::String(entry.to_string()))
                .collect(),
        )),
    }
}

fn upsert(root: &mut Value, path: &str, value: Value) {
    let mut current = root;
    let mut segments = path.split('.').peekable();
    while let Some(segment) = segments.next() {
        let object = current
            .as_object_mut()
            .expect("environment provider root and intermediate values are objects");
        if segments.peek().is_none() {
            object.insert(segment.to_string(), value);
            return;
        }
        current = object
            .entry(segment)
            .or_insert_with(|| Value::Object(serde_json::Map::new()));
    }
}

macro_rules! impl_provider {
    ($provider:ty, $name:literal) => {
        impl Provider for $provider {
            fn metadata(&self) -> Metadata {
                Metadata::named($name)
            }

            fn data(&self) -> Result<Map<Profile, Dict>, Error> {
                Serialized::defaults(&self.values).data()
            }
        }
    };
}

impl_provider!(ParControlEnvProvider, "par-control environment variables");

#[cfg(test)]
mod tests {
    use super::*;

    fn lookup<'a>(vars: &'a [(&str, &str)]) -> impl Fn(&str) -> Option<String> + 'a {
        |name| {
            vars.iter()
                .find(|(candidate, _)| *candidate == name)
                .map(|(_, value)| (*value).to_string())
        }
    }

    #[test]
    fn providers_decode_typed_values_and_aliases() {
        let normal = [
            ("DD_DD_URL", "https://preferred.example"),
            ("DD_URL", "https://fallback.example"),
            ("DD_PRIVATE_ACTION_RUNNER_ENABLED", "true"),
            ("DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY", "7"),
            (
                "DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS",
                r#"{"X-Test":"value"}"#,
            ),
        ];
        let values = ParControlEnvProvider::new(&lookup(&normal)).unwrap().values;
        assert_eq!(
            values.pointer("/dd_url"),
            Some(&serde_json::json!("https://preferred.example"))
        );
        assert_eq!(
            values.pointer("/private_action_runner/enabled"),
            Some(&serde_json::json!(true))
        );
        assert_eq!(
            values.pointer("/private_action_runner/task_concurrency"),
            Some(&serde_json::json!(7))
        );
        assert_eq!(
            values.pointer("/private_action_runner/opms_extra_headers/X-Test"),
            Some(&serde_json::json!("value"))
        );

        let proxy = [
            ("HTTP_PROXY", "http://proxy.example"),
            ("NO_PROXY", "one.example, two.example"),
        ];
        let values = ParControlEnvProvider::new(&lookup(&proxy)).unwrap().values;
        assert_eq!(
            values.pointer("/proxy/http"),
            Some(&serde_json::json!("http://proxy.example"))
        );
        assert_eq!(
            values.pointer("/proxy/no_proxy"),
            Some(&serde_json::json!(["one.example", "two.example"]))
        );
    }

    #[test]
    fn malformed_typed_values_are_rejected() {
        for (name, value) in [
            ("DD_PRIVATE_ACTION_RUNNER_TASK_CONCURRENCY", "many"),
            ("DD_PRIVATE_ACTION_RUNNER_OPMS_EXTRA_HEADERS", "[]"),
        ] {
            let vars = [(name, value)];
            assert!(ParControlEnvProvider::new(&lookup(&vars)).is_err());
        }
    }
}
