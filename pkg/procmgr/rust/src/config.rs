// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config_gate::{ConditionConfigFile, HostOs};
use crate::platform;
use crate::spawn::{RequestedIdentity, translate};
use anyhow::{Context, Result};
use log::{debug, info, warn};
use serde::Deserialize;
use std::collections::{HashMap, HashSet};
use std::fmt;
use std::path::{Path, PathBuf};
use std::time::Duration;

pub struct ProcessDefinition {
    pub name: String,
    pub config: ProcessConfig,
}

/// A `processes.d` file that could not be turned into a `ProcessConfig`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct InvalidConfigEntry {
    pub name: String,
    pub path: PathBuf,
    pub error: String,
}

const CONFIG_ERROR_TRUNCATED_SUFFIX: &str = "… (truncated)";

pub const STORED_CONFIG_ERROR_MAX_CHARS: usize = 4096;
pub const LIST_CONFIG_ERROR_MAX_CHARS: usize = 256;

pub fn truncate_config_error(error: &str, max_chars: usize) -> String {
    let mut iter = error.chars();
    let head: String = iter.by_ref().take(max_chars).collect();
    if iter.next().is_none() {
        return head;
    }
    let suffix_len = CONFIG_ERROR_TRUNCATED_SUFFIX.chars().count();
    let keep = max_chars.saturating_sub(suffix_len);
    let trimmed: String = head.chars().take(keep).collect();
    format!("{trimmed}{CONFIG_ERROR_TRUNCATED_SUFFIX}")
}

/// Result of one catalog load: valid process configs plus named parse failures.
#[derive(Default)]
pub struct LoadedCatalog {
    pub processes: Vec<ProcessDefinition>,
    pub invalid: Vec<InvalidConfigEntry>,
}

impl LoadedCatalog {
    pub fn valid(processes: Vec<ProcessDefinition>) -> Self {
        Self {
            processes,
            invalid: Vec::new(),
        }
    }
}

pub trait ConfigLoader: Send + Sync {
    fn load(&self) -> LoadedCatalog;
    /// Like [`ConfigLoader::load`], but a failed or stalled read is an error, so a reload keeps
    /// the current processes.
    fn try_load(&self) -> Result<LoadedCatalog> {
        Ok(self.load())
    }
    fn source(&self) -> &str;
    fn location(&self) -> String;
}

#[cfg(test)]
pub struct StaticConfigLoader(LoadedCatalog);

#[cfg(test)]
impl StaticConfigLoader {
    pub fn new(configs: Vec<ProcessDefinition>) -> Self {
        Self(LoadedCatalog::valid(configs))
    }

    pub fn with_catalog(catalog: LoadedCatalog) -> Self {
        Self(catalog)
    }
}

#[cfg(test)]
impl ConfigLoader for StaticConfigLoader {
    fn load(&self) -> LoadedCatalog {
        LoadedCatalog {
            processes: self
                .0
                .processes
                .iter()
                .map(|pd| ProcessDefinition {
                    name: pd.name.clone(),
                    config: pd.config.clone(),
                })
                .collect(),
            invalid: self.0.invalid.clone(),
        }
    }

    fn source(&self) -> &str {
        "static"
    }

    fn location(&self) -> String {
        "in-memory (test)".to_string()
    }
}

#[cfg(test)]
pub struct MutableConfigLoader {
    catalog: std::sync::RwLock<LoadedCatalog>,
}

#[cfg(test)]
impl MutableConfigLoader {
    pub fn new(configs: Vec<ProcessDefinition>) -> Self {
        Self {
            catalog: std::sync::RwLock::new(LoadedCatalog::valid(configs)),
        }
    }

    pub fn set(&self, configs: Vec<ProcessDefinition>) {
        self.set_catalog(LoadedCatalog::valid(configs));
    }

    pub fn set_catalog(&self, catalog: LoadedCatalog) {
        *self.catalog.write().unwrap() = catalog;
    }
}

#[cfg(test)]
impl ConfigLoader for MutableConfigLoader {
    fn load(&self) -> LoadedCatalog {
        let catalog = self.catalog.read().unwrap();
        LoadedCatalog {
            processes: catalog
                .processes
                .iter()
                .map(|pd| ProcessDefinition {
                    name: pd.name.clone(),
                    config: pd.config.clone(),
                })
                .collect(),
            invalid: catalog.invalid.clone(),
        }
    }

    fn source(&self) -> &str {
        "mutable"
    }

    fn location(&self) -> String {
        "in-memory (mutable test)".to_string()
    }
}

pub struct YamlConfigLoader {
    dir: PathBuf,
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum DirKind {
    Dir,
    NotDir,
    DanglingLink,
    Missing,
}

/// What `dir` is. It is absent only on `NotFound` with no dangling link in its path.
fn classify_dir(dir: &Path) -> std::io::Result<DirKind> {
    use std::io::ErrorKind::NotFound;
    match std::fs::metadata(dir) {
        Ok(meta) if meta.is_dir() => return Ok(DirKind::Dir),
        Ok(_) => return Ok(DirKind::NotDir),
        Err(err) if err.kind() == NotFound => {}
        Err(err) => return Err(err),
    }
    // A dangling link, as the path or a parent (replaced during a package switch), is unreadable
    // for now, not absent. Look at the deepest component that exists.
    for candidate in dir.ancestors() {
        match std::fs::symlink_metadata(candidate) {
            Ok(meta) => {
                let dangling = meta.file_type().is_symlink()
                    && matches!(std::fs::metadata(candidate),
                        Err(err) if err.kind() == NotFound);
                return Ok(if dangling {
                    DirKind::DanglingLink
                } else {
                    DirKind::Missing
                });
            }
            Err(err) if err.kind() == NotFound => {}
            Err(err) => return Err(err),
        }
    }
    Ok(DirKind::Missing)
}

impl YamlConfigLoader {
    pub fn from_env() -> Self {
        Self { dir: config_dir() }
    }

    /// Startup loads leniently and reload strictly (see `load_configs_with`). A strict load also
    /// fails on a non-directory or a dangling link.
    fn load_with(&self, strict: bool) -> Result<LoadedCatalog> {
        let owned = self.dir.clone();
        let kind = crate::stop::bounded(&self.dir, move || classify_dir(&owned))
            .and_then(|result| result)
            .with_context(|| format!("cannot check config directory {}", self.dir.display()))?;
        let dir = self.dir.display();
        let problem = match kind {
            DirKind::Dir => None,
            DirKind::Missing => Some(format!("config directory {dir} does not exist")),
            DirKind::NotDir => Some(format!("config path {dir} is not a directory")),
            DirKind::DanglingLink => Some(format!("config path {dir} is a dangling link")),
        };
        if let Some(problem) = problem {
            if strict && kind != DirKind::Missing {
                anyhow::bail!("{problem}");
            }
            info!("{problem}, no processes to manage");
            return Ok(LoadedCatalog::default());
        }

        let catalog = load_configs_with(&self.dir, strict)?;
        info!(
            "loaded {} process config(s) ({} invalid) from {}",
            catalog.processes.len(),
            catalog.invalid.len(),
            self.dir.display()
        );
        Ok(catalog)
    }
}

impl ConfigLoader for YamlConfigLoader {
    fn source(&self) -> &str {
        "yaml"
    }

    fn location(&self) -> String {
        self.dir.display().to_string()
    }

    fn load(&self) -> LoadedCatalog {
        self.load_with(false).unwrap_or_else(|err| {
            warn!(
                "cannot read config directory {}: {err:#}",
                self.dir.display()
            );
            LoadedCatalog::default()
        })
    }

    fn try_load(&self) -> Result<LoadedCatalog> {
        self.load_with(true)
    }
}

fn default_true() -> bool {
    true
}

fn default_inherit() -> String {
    "inherit".to_string()
}

fn default_restart() -> RestartPolicy {
    RestartPolicy::Never
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum RestartPolicy {
    Always,
    OnFailure,
    OnSuccess,
    Never,
}

impl fmt::Display for RestartPolicy {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            RestartPolicy::Always => write!(f, "always"),
            RestartPolicy::OnFailure => write!(f, "on-failure"),
            RestartPolicy::OnSuccess => write!(f, "on-success"),
            RestartPolicy::Never => write!(f, "never"),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct ProcessConfig {
    #[serde(default)]
    #[allow(dead_code)]
    pub description: Option<String>,
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub env: HashMap<String, String>,
    pub environment_file: Option<String>,
    pub working_dir: Option<String>,
    /// Parsed for forward-compatibility but not yet acted upon.
    #[allow(dead_code)]
    pub pidfile: Option<String>,
    #[serde(default = "default_inherit")]
    pub stdout: String,
    #[serde(default = "default_inherit")]
    pub stderr: String,
    #[serde(default = "default_true")]
    pub auto_start: bool,
    pub condition_path_exists: Option<String>,
    #[serde(default)]
    pub condition_config_any: Vec<ConditionConfigFile>,
    /// Keys that must all be false for the process to start. Vetoes the any-of above.
    #[serde(default)]
    pub condition_config_none: Vec<ConditionConfigFile>,
    pub stop_timeout: Option<u64>,
    #[serde(default = "default_restart")]
    pub restart: RestartPolicy,
    pub restart_sec: Option<f64>,
    pub restart_max_delay_sec: Option<f64>,
    pub start_limit_burst: Option<u32>,
    pub start_limit_interval_sec: Option<u64>,
    pub runtime_success_sec: Option<u64>,
    #[serde(default)]
    pub after: Vec<String>,
    #[serde(default)]
    pub before: Vec<String>,
    /// Exit the daemon with this process's code (incl. 0). Overrides `restart`.
    #[serde(default)]
    pub exit_with_supervisor: bool,
    // Crate-private: ProcessConfig is public and RequestedIdentity is not.
    #[serde(default)]
    pub(crate) identity: RequestedIdentity,
}

const DEFAULT_STOP_TIMEOUT_SECS: u64 = 90;
const DEFAULT_RESTART_SEC: f64 = 1.0;
const DEFAULT_RESTART_MAX_DELAY_SEC: f64 = 60.0;
const DEFAULT_START_LIMIT_BURST: u32 = 5;
const DEFAULT_START_LIMIT_INTERVAL_SEC: u64 = 10;
const DEFAULT_RUNTIME_SUCCESS_SEC: u64 = 1;

impl Default for ProcessConfig {
    fn default() -> Self {
        Self {
            description: None,
            command: String::new(),
            args: vec![],
            env: HashMap::new(),
            environment_file: None,
            working_dir: None,
            pidfile: None,
            stdout: "inherit".to_string(),
            stderr: "inherit".to_string(),
            auto_start: true,
            condition_path_exists: None,
            condition_config_any: Vec::new(),
            condition_config_none: Vec::new(),
            stop_timeout: None,
            restart: RestartPolicy::Never,
            restart_sec: None,
            restart_max_delay_sec: None,
            start_limit_burst: None,
            start_limit_interval_sec: None,
            runtime_success_sec: None,
            after: vec![],
            before: vec![],
            exit_with_supervisor: false,
            identity: RequestedIdentity::default(),
        }
    }
}

impl ProcessConfig {
    pub fn stop_timeout(&self) -> Duration {
        Duration::from_secs(self.stop_timeout.unwrap_or(DEFAULT_STOP_TIMEOUT_SECS))
    }

    pub fn restart_delay(&self) -> f64 {
        self.restart_sec.unwrap_or(DEFAULT_RESTART_SEC)
    }

    pub fn max_restart_delay(&self) -> f64 {
        self.restart_max_delay_sec
            .unwrap_or(DEFAULT_RESTART_MAX_DELAY_SEC)
    }

    pub fn burst_limit(&self) -> u32 {
        self.start_limit_burst.unwrap_or(DEFAULT_START_LIMIT_BURST)
    }

    pub fn burst_interval(&self) -> Duration {
        Duration::from_secs(
            self.start_limit_interval_sec
                .unwrap_or(DEFAULT_START_LIMIT_INTERVAL_SEC),
        )
    }

    pub fn runtime_success(&self) -> Duration {
        Duration::from_secs(
            self.runtime_success_sec
                .unwrap_or(DEFAULT_RUNTIME_SUCCESS_SEC),
        )
    }
}

pub fn config_dir() -> PathBuf {
    std::env::var("DD_PM_CONFIG_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|_| platform::default_config_dir())
}

/// Scan a directory for `*.yaml` files and parse each into a ProcessConfig.
/// The process name is derived from the filename (without extension).
/// Files that fail to parse are logged and returned as invalid catalog entries.
pub fn load_configs(dir: &Path) -> Result<LoadedCatalog> {
    load_configs_with(dir, false)
}

/// `strict` is the reload mode: a stalled read or an unreadable entry fails the whole load,
/// since treating it as invalid or missing would stop a running process.
fn load_configs_with(dir: &Path, strict: bool) -> Result<LoadedCatalog> {
    // Entries are listed and stat-ed on the helper thread, since iterating a ReadDir blocks on a
    // stalled mount. A FIFO, socket or device would block a read for good, so it is an invalid
    // entry in both modes. A failed stat leaves the entry to the read to report.
    let owned = dir.to_path_buf();
    let entries = crate::stop::bounded(dir, move || {
        std::fs::read_dir(owned).map(|rd| {
            rd.map(|entry| {
                entry.map(|entry| {
                    let regular = is_yaml_name(&entry.path())
                        .then(|| std::fs::metadata(entry.path()).map(|m| m.is_file()).ok())
                        .flatten();
                    (entry, regular)
                })
            })
            .collect::<Vec<_>>()
        })
    })
    .and_then(|result| result)
    .with_context(|| format!("failed to read config directory: {}", dir.display()))?;

    let mut listed = Vec::new();
    for entry in entries {
        match entry {
            Ok(entry) => listed.push(entry),
            Err(e) if strict => {
                return Err(anyhow::Error::new(e)
                    .context(format!("failed to read an entry in {}", dir.display())));
            }
            Err(e) => warn!("skipping unreadable entry in {}: {e}", dir.display()),
        }
    }
    let mut yaml_files: Vec<_> = listed
        .into_iter()
        .filter(|(e, _)| {
            let is_yaml = is_yaml_name(&e.path());
            if !is_yaml {
                debug!("skipping non-YAML file: {}", e.path().display());
            }
            is_yaml
        })
        .collect();

    yaml_files.sort_by_key(|(e, _)| e.file_name());

    let mut processes = Vec::new();
    let mut invalid = Vec::new();
    for (entry, regular) in yaml_files {
        if !strict && crate::stop::stop_requested_flag() {
            break;
        }
        let path = entry.path();
        let name = path
            .file_stem()
            .and_then(|s| s.to_str())
            .unwrap_or("unknown")
            .to_string();

        let parsed = if regular == Some(false) {
            Err(anyhow::anyhow!("{} is not a regular file", path.display()))
        } else {
            parse_config(&path)
        };
        match parsed {
            Ok(config) => processes.push(ProcessDefinition { name, config }),
            Err(e) if strict && is_stalled_read(&e) => return Err(e),
            Err(e) => {
                let error = truncate_config_error(&format!("{e:#}"), STORED_CONFIG_ERROR_MAX_CHARS);
                warn!("[{name}] skipping config: {error}");
                invalid.push(InvalidConfigEntry { name, path, error });
            }
        }
    }

    // `.yaml` and `.yml` share a file stem, so sibling files claim one catalog name. A valid file
    // wins because it is spawnable and a stub of its name would shadow it in describe and start.
    // Between two broken siblings the first of the sorted scan wins, which keeps one row per name
    // instead of two that only one of them can ever be reached through.
    let mut claimed: HashSet<String> = processes.iter().map(|p| p.name.clone()).collect();
    invalid.retain(|entry| {
        if claimed.insert(entry.name.clone()) {
            return true;
        }
        warn!(
            "[{}] ignoring {}: another config of the same name was loaded",
            entry.name,
            entry.path.display()
        );
        false
    });

    Ok(LoadedCatalog { processes, invalid })
}

/// A config read that the bounded call gave up on or could not run.
#[derive(Debug)]
struct UnfinishedRead(std::io::Error);

impl fmt::Display for UnfinishedRead {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(&self.0, f)
    }
}

impl std::error::Error for UnfinishedRead {}

/// Whether a config read did not finish or timed out. It may succeed later, so a reload refuses
/// it. Any other read error is an invalid entry, as a parse error is.
fn is_stalled_read(err: &anyhow::Error) -> bool {
    err.downcast_ref::<UnfinishedRead>().is_some()
        || err.downcast_ref::<std::io::Error>().is_some_and(|io| {
            matches!(
                io.kind(),
                std::io::ErrorKind::TimedOut | std::io::ErrorKind::Interrupted
            )
        })
}

fn is_yaml_name(path: &Path) -> bool {
    path.extension()
        .is_some_and(|ext| ext == "yaml" || ext == "yml")
}

fn parse_config(path: &Path) -> Result<ProcessConfig> {
    let owned = path.to_path_buf();
    let read = crate::stop::bounded(path, move || {
        #[cfg(test)]
        crate::stop::testing::park_if_stalled(&owned);
        std::fs::read_to_string(owned)
    })
    .map_err(UnfinishedRead)
    .with_context(|| format!("reading {}", path.display()))?;
    let contents = read.with_context(|| format!("reading {}", path.display()))?;
    let config: ProcessConfig =
        serde_yaml::from_str(&contents).with_context(|| format!("parsing {}", path.display()))?;
    validate_identity(&config.identity, HostOs::CURRENT)
        .with_context(|| format!("parsing {}", path.display()))?;
    Ok(config)
}

fn validate_identity(identity: &RequestedIdentity, os: HostOs) -> Result<()> {
    translate(identity, os)
        .map(|_| ())
        .map_err(anyhow::Error::msg)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[cfg(unix)]
    fn mkfifo(path: &Path) {
        let c = std::ffi::CString::new(path.to_str().unwrap()).unwrap();
        assert_eq!(unsafe { nix::libc::mkfifo(c.as_ptr(), 0o600) }, 0, "mkfifo");
    }

    /// A regular config file whose read blocks, stalled through the `stop` test seam.
    fn stalled_config(dir: &Path, name: &str) {
        let path = dir.join(name);
        fs::write(&path, "command: /usr/bin/true\n").unwrap();
        crate::stop::testing::stall(&path);
    }

    /// A FIFO entry is invalid without being opened, at startup and on reload.
    #[cfg(unix)]
    #[test]
    fn a_fifo_entry_is_invalid_without_being_opened() {
        let dir = tempfile::tempdir().unwrap();
        let fifo = dir.path().join("blocked.yaml");
        mkfifo(&fifo);
        std::os::unix::fs::symlink(&fifo, dir.path().join("linked.yaml")).unwrap();
        fs::write(dir.path().join("good.yaml"), "command: /usr/bin/true\n").unwrap();
        let loader = YamlConfigLoader {
            dir: dir.path().to_path_buf(),
        };

        let started = std::time::Instant::now();
        let reload = loader.try_load().expect("a FIFO must not fail a reload");
        let startup = loader.load();
        assert!(
            started.elapsed() < crate::stop::CALL_DEADLINE,
            "a FIFO entry was opened"
        );
        for catalog in [reload, startup] {
            assert_eq!(catalog.processes.len(), 1, "the valid config must load");
            let mut names: Vec<_> = catalog.invalid.iter().map(|e| e.name.as_str()).collect();
            names.sort();
            assert_eq!(names, ["blocked", "linked"]);
            assert!(
                catalog
                    .invalid
                    .iter()
                    .all(|e| e.error.contains("not a regular file")),
                "{:?}",
                catalog.invalid.iter().map(|e| &e.error).collect::<Vec<_>>()
            );
        }
    }

    /// On reload a stalled config file fails the whole load, not just that entry.
    #[test]
    fn a_stalled_config_file_fails_a_reload_load() {
        let dir = tempfile::tempdir().unwrap();
        stalled_config(dir.path(), "blocked.yaml");

        let loader = YamlConfigLoader {
            dir: dir.path().to_path_buf(),
        };
        let err = match loader.try_load() {
            Ok(_) => panic!("a stalled read must fail the reload load"),
            Err(err) => err,
        };
        let unfinished = err
            .downcast_ref::<UnfinishedRead>()
            .expect("the stall must stay an unfinished read");
        assert_eq!(unfinished.0.kind(), std::io::ErrorKind::TimedOut);
    }

    /// A read the bounded call could not run fails a reload. An error from the read itself does
    /// only as `TimedOut` or `Interrupted`.
    #[test]
    fn only_a_read_that_did_not_finish_fails_a_reload() {
        let kind = std::io::Error::from;
        let would_block = || kind(std::io::ErrorKind::WouldBlock);
        let unfinished = anyhow::Error::new(UnfinishedRead(would_block())).context("reading x");
        assert!(is_stalled_read(&unfinished));
        let failed = anyhow::Error::new(would_block()).context("reading x");
        assert!(!is_stalled_read(&failed));
        for stalled in [
            std::io::ErrorKind::TimedOut,
            std::io::ErrorKind::Interrupted,
        ] {
            let err = anyhow::Error::new(kind(stalled)).context("reading x");
            assert!(is_stalled_read(&err), "{stalled:?} from the read can clear");
        }
    }

    /// At startup the same stall marks the file invalid and keeps the rest of the catalog.
    #[test]
    fn a_stalled_config_file_is_invalid_at_startup() {
        let dir = tempfile::tempdir().unwrap();
        stalled_config(dir.path(), "blocked.yaml");
        fs::write(dir.path().join("good.yaml"), "command: /usr/bin/true\n").unwrap();

        let catalog = YamlConfigLoader {
            dir: dir.path().to_path_buf(),
        }
        .load();
        assert_eq!(
            catalog.processes.len(),
            1,
            "the valid config must still load"
        );
        assert_eq!(catalog.invalid.len(), 1);
    }

    /// A config path that is a file reads as an empty catalog at startup, and fails a reload.
    #[test]
    fn a_config_path_that_is_a_file_fails_only_a_reload() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("processes.d");
        fs::write(&file, "").unwrap();
        let loader = YamlConfigLoader { dir: file };

        assert!(
            loader.try_load().is_err(),
            "a reload must refuse a non-directory"
        );
        let catalog = loader.load();
        assert!(catalog.processes.is_empty() && catalog.invalid.is_empty());
    }

    #[cfg(unix)]
    #[test]
    fn a_dangling_config_link_fails_only_a_reload() {
        let dir = tempfile::tempdir().unwrap();
        let link = dir.path().join("processes.d");
        std::os::unix::fs::symlink(dir.path().join("gone"), &link).unwrap();
        let loader = YamlConfigLoader { dir: link };

        assert!(
            loader.try_load().is_err(),
            "a reload must refuse a dangling link"
        );
        let catalog = loader.load();
        assert!(catalog.processes.is_empty() && catalog.invalid.is_empty());
    }

    /// A dangling parent link fails a reload. A missing directory under a live link is absent.
    #[cfg(unix)]
    #[test]
    fn a_dangling_parent_link_fails_only_a_reload() {
        let root = tempfile::tempdir().unwrap();
        let link = root.path().join("current");
        std::os::unix::fs::symlink(root.path().join("gone"), &link).unwrap();
        let loader = YamlConfigLoader {
            dir: link.join("processes.d"),
        };
        assert!(
            loader.try_load().is_err(),
            "a reload must refuse a dangling parent link"
        );
        let catalog = loader.load();
        assert!(catalog.processes.is_empty() && catalog.invalid.is_empty());

        let real = root.path().join("real");
        fs::create_dir(&real).unwrap();
        let resolving = root.path().join("resolving");
        std::os::unix::fs::symlink(&real, &resolving).unwrap();
        let loader = YamlConfigLoader {
            dir: resolving.join("processes.d"),
        };
        let catalog = loader
            .try_load()
            .expect("a missing directory under a resolving link is absent");
        assert!(catalog.processes.is_empty() && catalog.invalid.is_empty());
    }

    /// A read error that does not clear on its own is an invalid entry on reload too.
    #[test]
    fn a_permanently_unreadable_config_is_invalid_on_reload() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("bad.yaml"), b"command: \xff\xfe\n").unwrap();
        fs::write(dir.path().join("good.yaml"), "command: /bin/true\n").unwrap();
        let loader = YamlConfigLoader {
            dir: dir.path().to_path_buf(),
        };

        let catalog = match loader.try_load() {
            Ok(catalog) => catalog,
            Err(err) => panic!("a reload must not refuse invalid UTF-8: {err:#}"),
        };
        assert_eq!(catalog.processes.len(), 1);
        assert_eq!(catalog.invalid.len(), 1);
        assert_eq!(catalog.invalid[0].name, "bad");
    }

    #[test]
    fn test_parse_full_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = r#"
description: Test Process
command: /usr/bin/sleep
args:
  - "9999"
env:
  FOO: bar
  BAZ: qux
working_dir: /tmp
pidfile: /tmp/test.pid
stdout: inherit
stderr: inherit
auto_start: true
condition_path_exists: /usr/bin/sleep
"#;
        fs::write(dir.path().join("test-proc.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap().processes;
        assert_eq!(configs.len(), 1);

        let np = &configs[0];
        assert_eq!(np.name, "test-proc");
        assert_eq!(np.config.command, "/usr/bin/sleep");
        assert_eq!(np.config.args, vec!["9999"]);
        assert_eq!(np.config.env.get("FOO").unwrap(), "bar");
        assert_eq!(np.config.working_dir.as_deref(), Some("/tmp"));
        assert_eq!(np.config.pidfile.as_deref(), Some("/tmp/test.pid"));
        assert!(np.config.auto_start);
        assert_eq!(
            np.config.condition_path_exists.as_deref(),
            Some("/usr/bin/sleep")
        );
    }

    #[test]
    fn test_parse_minimal_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = "command: /usr/bin/true\n";
        fs::write(dir.path().join("minimal.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap().processes;
        assert_eq!(configs.len(), 1);

        let np = &configs[0];
        assert_eq!(np.name, "minimal");
        assert_eq!(np.config.command, "/usr/bin/true");
        assert!(np.config.args.is_empty());
        assert!(np.config.env.is_empty());
        assert!(np.config.auto_start);
        assert_eq!(np.config.stdout, "inherit");
        assert_eq!(np.config.stderr, "inherit");
        assert!(np.config.condition_path_exists.is_none());
    }

    #[test]
    fn test_records_invalid_files() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("good.yaml"), "command: /usr/bin/true\n").unwrap();
        fs::write(dir.path().join("bad.yaml"), "not: valid: yaml: [").unwrap();
        fs::write(
            dir.path().join("no-command.yaml"),
            "description: missing command\n",
        )
        .unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert_eq!(catalog.processes.len(), 1);
        assert_eq!(catalog.processes[0].name, "good");
        assert_eq!(catalog.invalid.len(), 2);
        let names: Vec<&str> = catalog.invalid.iter().map(|e| e.name.as_str()).collect();
        assert_eq!(names, vec!["bad", "no-command"]);
        for entry in &catalog.invalid {
            assert!(
                !entry.error.is_empty(),
                "invalid entry {} should carry a parse error",
                entry.name
            );
            assert_eq!(entry.path, dir.path().join(format!("{}.yaml", entry.name)));
        }
    }

    #[test]
    fn test_invalid_sibling_does_not_shadow_a_valid_config() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("svc.yaml"), "command: /usr/bin/true\n").unwrap();
        fs::write(dir.path().join("svc.yml"), "not: valid: yaml: [").unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert_eq!(catalog.processes.len(), 1);
        assert_eq!(catalog.processes[0].name, "svc");
        assert!(
            catalog.invalid.is_empty(),
            "a spawnable config must win over a broken file sharing its name"
        );
    }

    #[test]
    fn test_truncate_config_error_keeps_short_text() {
        let err = "missing field `command`";
        assert_eq!(truncate_config_error(err, LIST_CONFIG_ERROR_MAX_CHARS), err);
    }

    #[test]
    fn test_truncate_config_error_respects_budget() {
        let err = "a".repeat(LIST_CONFIG_ERROR_MAX_CHARS + 50);
        let out = truncate_config_error(&err, LIST_CONFIG_ERROR_MAX_CHARS);
        assert_eq!(out.chars().count(), LIST_CONFIG_ERROR_MAX_CHARS);
        assert!(out.ends_with(CONFIG_ERROR_TRUNCATED_SUFFIX));
    }

    #[test]
    fn test_load_configs_caps_stored_parse_error() {
        let dir = tempfile::tempdir().unwrap();
        let huge = "z".repeat(STORED_CONFIG_ERROR_MAX_CHARS + 8_000);
        fs::write(
            dir.path().join("huge.yaml"),
            format!("command: /a\nargs: \"{huge}\"\n"),
        )
        .unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert_eq!(catalog.invalid.len(), 1);
        let error = &catalog.invalid[0].error;
        assert!(
            error.chars().count() <= STORED_CONFIG_ERROR_MAX_CHARS,
            "stored parse error must be capped, got {} chars",
            error.chars().count()
        );
        assert!(
            error.contains("truncated"),
            "capped diagnostics must mark the cut: {error}"
        );
    }

    /// The flare scrubber in pkg/procmgr/coat strips these echoed values when an operator asks for
    /// arguments to be stripped, and it recognizes them by the way serde quotes them. The spelling
    /// is therefore a contract between the two, not an implementation detail of this crate.
    #[test]
    fn test_rejected_scalars_are_echoed_in_the_parse_error() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(
            dir.path().join("text.yaml"),
            "command: /a\nargs: \"--token abc123\"\n",
        )
        .unwrap();
        fs::write(
            dir.path().join("number.yaml"),
            "command: /a\nargs: 1234567\n",
        )
        .unwrap();

        let errors: Vec<String> = load_configs(dir.path())
            .unwrap()
            .invalid
            .into_iter()
            .map(|e| e.error)
            .collect();

        assert!(
            errors
                .iter()
                .any(|e| e.contains("invalid type: string \"--token abc123\"")),
            "{errors:?}"
        );
        assert!(
            errors
                .iter()
                .any(|e| e.contains("invalid type: integer `1234567`")),
            "{errors:?}"
        );
    }

    /// Two broken siblings claim the same catalog name, and only the first of them could ever be
    /// described or counted as itself, so the second is not a row worth having.
    #[test]
    fn test_invalid_siblings_collapse_to_one_entry() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("svc.yaml"), "not: valid: yaml: [").unwrap();
        fs::write(dir.path().join("svc.yml"), "description: no command\n").unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert!(catalog.processes.is_empty());
        assert_eq!(catalog.invalid.len(), 1);
        assert_eq!(catalog.invalid[0].name, "svc");
        assert_eq!(
            catalog.invalid[0].path,
            dir.path().join("svc.yaml"),
            "the winner should follow the sorted scan rather than readdir order"
        );
    }

    #[test]
    fn test_empty_directory() {
        let dir = tempfile::tempdir().unwrap();
        let catalog = load_configs(dir.path()).unwrap();
        assert!(catalog.processes.is_empty());
        assert!(catalog.invalid.is_empty());
    }

    #[test]
    fn test_sorted_alphabetically() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("charlie.yaml"), "command: /c\n").unwrap();
        fs::write(dir.path().join("alpha.yaml"), "command: /a\n").unwrap();
        fs::write(dir.path().join("bravo.yaml"), "command: /b\n").unwrap();

        let configs = load_configs(dir.path()).unwrap().processes;
        let names: Vec<&str> = configs.iter().map(|np| np.name.as_str()).collect();
        assert_eq!(names, vec!["alpha", "bravo", "charlie"]);
    }

    #[test]
    fn test_ignores_non_yaml() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("proc.yaml"), "command: /a\n").unwrap();
        fs::write(dir.path().join("readme.txt"), "not a config").unwrap();
        fs::write(dir.path().join("notes.md"), "also not").unwrap();

        let configs = load_configs(dir.path()).unwrap().processes;
        assert_eq!(configs.len(), 1);
    }

    #[test]
    fn test_auto_start_defaults_true() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("p.yaml"), "command: /a\n").unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert!(configs[0].config.auto_start);
    }

    #[test]
    fn test_auto_start_false() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(
            dir.path().join("p.yaml"),
            "command: /a\nauto_start: false\n",
        )
        .unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert!(!configs[0].config.auto_start);
    }

    #[test]
    fn test_exit_with_supervisor_defaults_false() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("p.yaml"), "command: /a\n").unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert!(!configs[0].config.exit_with_supervisor);
    }

    #[test]
    fn test_exit_with_supervisor_true() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(
            dir.path().join("p.yaml"),
            "command: /a\nexit_with_supervisor: true\n",
        )
        .unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert!(configs[0].config.exit_with_supervisor);
    }

    #[test]
    fn test_load_configs_nonexistent_directory() {
        let result = load_configs(Path::new("/nonexistent/processes.d"));
        assert!(result.is_err());
    }

    #[cfg(unix)]
    #[test]
    fn test_load_configs_unreadable_directory() {
        use nix::unistd::Uid;
        use std::os::unix::fs::PermissionsExt;

        if Uid::effective().is_root() {
            eprintln!("skipping test_load_configs_unreadable_directory: running as root");
            return;
        }

        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("proc.yaml"), "command: /a\n").unwrap();
        fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o000)).unwrap();

        let result = load_configs(dir.path());
        fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o755)).unwrap();
        assert!(result.is_err());
    }

    #[test]
    fn test_process_agent_config_gate_parsing() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = r#"
command: /bin/process-agent
auto_start: true
condition_config_any:
  - path: /etc/datadog-agent/datadog.yaml
    keys:
      - process_config.enabled
      - process_config.process_collection.enabled
  - path: /etc/datadog-agent/system-probe.yaml
    keys:
      - network_config.enabled
"#;
        fs::write(dir.path().join("proc.yaml"), yaml).unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert_eq!(configs.len(), 1);
        assert_eq!(configs[0].config.condition_config_any.len(), 2);
        assert_eq!(
            configs[0].config.condition_config_any[0].keys,
            vec![
                "process_config.enabled".to_string(),
                "process_config.process_collection.enabled".to_string(),
            ]
        );
        assert_eq!(
            configs[0].config.condition_config_any[1].path,
            "/etc/datadog-agent/system-probe.yaml"
        );
    }

    #[test]
    fn test_config_gate_defaults_to_empty() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("p.yaml"), "command: /a\n").unwrap();
        let configs = load_configs(dir.path()).unwrap().processes;
        assert!(
            configs[0].config.condition_config_any.is_empty(),
            "a yaml without condition_config_any should leave the gate open"
        );
    }

    #[test]
    fn test_ddot_example_config() {
        let dir = tempfile::tempdir().unwrap();
        let yaml = r#"
description: Datadog Distribution of OpenTelemetry Collector
command: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
args:
  - run
  - --config
  - /etc/datadog-agent/otel-config.yaml
  - --core-config
  - /etc/datadog-agent/datadog.yaml
  - --pidfile
  - /opt/datadog-agent/run/otel-agent.pid
auto_start: true
condition_path_exists: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
env:
  DD_FLEET_POLICIES_DIR: /etc/datadog-agent/managed/datadog-agent/stable
stdout: inherit
stderr: inherit
"#;
        fs::write(dir.path().join("datadog-agent-ddot.yaml"), yaml).unwrap();

        let configs = load_configs(dir.path()).unwrap().processes;
        assert_eq!(configs.len(), 1);
        let np = &configs[0];
        assert_eq!(np.name, "datadog-agent-ddot");
        assert_eq!(
            np.config.command,
            "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"
        );
        assert_eq!(np.config.args.len(), 7);
        assert!(np.config.auto_start);
        assert_eq!(
            np.config.condition_path_exists.as_deref(),
            Some("/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent")
        );
    }

    #[test]
    fn test_absent_empty_and_bare_identity_are_the_default() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("absent.yaml"), "command: /bin/true\n").unwrap();
        fs::write(
            dir.path().join("empty.yaml"),
            "command: /bin/true\nidentity: {}\n",
        )
        .unwrap();
        fs::write(
            dir.path().join("bare.yaml"),
            "command: /bin/true\nidentity:\n",
        )
        .unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert!(
            catalog.invalid.is_empty(),
            "default identity forms must load: {:?}",
            catalog.invalid
        );
        assert_eq!(catalog.processes.len(), 3);
        let absent = &catalog.processes[0].config;
        let bare = &catalog.processes[1].config;
        let empty = &catalog.processes[2].config;
        assert_eq!(absent.identity, RequestedIdentity::default());
        assert_eq!(bare.identity, RequestedIdentity::default());
        assert_eq!(empty.identity, RequestedIdentity::default());
        assert_eq!(absent, bare);
        assert_eq!(bare, empty);
    }

    #[test]
    fn test_identity_parses_run_as_and_grants() {
        use crate::spawn::{Grant, RunAs};
        use std::collections::BTreeSet;

        let privileged: ProcessConfig =
            serde_yaml::from_str("command: /bin/true\nidentity:\n  run_as: privileged\n").unwrap();
        assert_eq!(
            privileged.identity,
            RequestedIdentity {
                run_as: RunAs::Privileged,
                grants: BTreeSet::new(),
            }
        );

        let bind: ProcessConfig = serde_yaml::from_str(
            "command: /bin/true\nidentity:\n  run_as: agent\n  grants: [bind-privileged-ports]\n",
        )
        .unwrap();
        assert_eq!(
            bind.identity,
            RequestedIdentity {
                run_as: RunAs::Agent,
                grants: BTreeSet::from([Grant::BindPrivilegedPorts]),
            }
        );

        let raw: ProcessConfig = serde_yaml::from_str(
            "command: /bin/true\nidentity:\n  run_as: agent\n  grants: [raw-network]\n",
        )
        .unwrap();
        assert_eq!(
            raw.identity,
            RequestedIdentity {
                run_as: RunAs::Agent,
                grants: BTreeSet::from([Grant::RawNetwork]),
            }
        );

        let bare_grants: ProcessConfig =
            serde_yaml::from_str("command: /bin/true\nidentity:\n  grants:\n").unwrap();
        assert_eq!(bare_grants.identity, RequestedIdentity::default());
    }

    #[test]
    fn test_identity_grant_order_does_not_change_the_config() {
        let forward: ProcessConfig = serde_yaml::from_str(
            "command: /bin/true\nidentity:\n  grants: [raw-network, bind-privileged-ports]\n",
        )
        .unwrap();
        let reverse: ProcessConfig = serde_yaml::from_str(
            "command: /bin/true\nidentity:\n  grants: [bind-privileged-ports, raw-network]\n",
        )
        .unwrap();
        assert_eq!(forward, reverse);
    }

    #[test]
    fn test_invalid_identity_is_invalid_config() {
        let dir = tempfile::tempdir().unwrap();
        let cases = [
            (
                "unknown-key.yaml",
                "command: /bin/true\nidentity:\n  extra: 1\n",
                "unknown field `extra`",
            ),
            (
                "unknown-run-as.yaml",
                "command: /bin/true\nidentity:\n  run_as: root\n",
                "unknown variant `root`",
            ),
            (
                "unknown-grant.yaml",
                "command: /bin/true\nidentity:\n  grants: [not-a-grant]\n",
                "unknown variant `not-a-grant`",
            ),
            (
                "duplicate-grant.yaml",
                "command: /bin/true\nidentity:\n  grants: [raw-network, raw-network]\n",
                "duplicate grant raw-network",
            ),
            (
                "null.yaml",
                "command: /bin/true\nidentity: null\n",
                "unit value, expected a mapping with optional run_as and grants",
            ),
            (
                "tilde.yaml",
                "command: /bin/true\nidentity: ~\n",
                "unit value, expected a mapping with optional run_as and grants",
            ),
            (
                "integer.yaml",
                "command: /bin/true\nidentity: 1\n",
                "integer `1`, expected a mapping with optional run_as and grants",
            ),
            (
                "privileged-grant.yaml",
                "command: /bin/true\nidentity:\n  run_as: privileged\n  grants: [bind-privileged-ports]\n",
                "identity: grants are not allowed with run_as privileged",
            ),
            (
                "sequence.yaml",
                "command: /bin/true\nidentity: [bind-privileged-ports]\n",
                "sequence, expected a mapping with optional run_as and grants",
            ),
            (
                "string.yaml",
                "command: /bin/true\nidentity: agent\n",
                "string \"agent\", expected a mapping with optional run_as and grants",
            ),
        ];
        for (name, yaml, _) in cases {
            fs::write(dir.path().join(name), yaml).unwrap();
        }

        let catalog = load_configs(dir.path()).unwrap();
        assert!(catalog.processes.is_empty());
        assert_eq!(catalog.invalid.len(), cases.len());
        for (name, _, needle) in cases {
            let stem = name.trim_end_matches(".yaml");
            let entry = catalog
                .invalid
                .iter()
                .find(|entry| entry.name == stem)
                .unwrap_or_else(|| panic!("missing invalid entry {stem}"));
            assert!(
                entry.error.contains(needle),
                "{stem} error should name the problem ({needle}): {}",
                entry.error
            );
        }
    }

    #[test]
    fn test_unknown_top_level_key_still_loads() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(
            dir.path().join("future.yaml"),
            "command: /bin/true\nfuture_key: 1\n",
        )
        .unwrap();

        let catalog = load_configs(dir.path()).unwrap();
        assert!(catalog.invalid.is_empty(), "{:?}", catalog.invalid);
        assert_eq!(catalog.processes.len(), 1);
        assert_eq!(catalog.processes[0].config.command, "/bin/true");
        assert_eq!(
            catalog.processes[0].config.identity,
            RequestedIdentity::default()
        );
    }

    #[test]
    fn test_validate_identity_for_every_host_os() {
        use crate::spawn::{Grant, RunAs};
        use std::collections::BTreeSet;

        // `Unavailable` is a grant the given `HostOs` cannot translate. `Unsupported` is
        // `run_as: privileged` on `HostOs::Other`.
        enum Expect {
            Ok,
            Unavailable(&'static str, &'static str),
            Unsupported,
        }

        let agent = RequestedIdentity::default();
        let bind = RequestedIdentity {
            run_as: RunAs::Agent,
            grants: BTreeSet::from([Grant::BindPrivilegedPorts]),
        };
        let raw = RequestedIdentity {
            run_as: RunAs::Agent,
            grants: BTreeSet::from([Grant::RawNetwork]),
        };
        let privileged = RequestedIdentity {
            run_as: RunAs::Privileged,
            grants: BTreeSet::new(),
        };

        let hosts = [HostOs::Linux, HostOs::MacOs, HostOs::Windows, HostOs::Other];
        for os in hosts {
            validate_identity(&agent, os)
                .unwrap_or_else(|err| panic!("agent with no grants should pass on {os:?}: {err}"));
        }

        let cases = [
            (HostOs::Linux, &bind, Expect::Ok),
            (
                HostOs::MacOs,
                &bind,
                Expect::Unavailable("bind-privileged-ports", "macos"),
            ),
            (HostOs::Windows, &bind, Expect::Ok),
            (
                HostOs::Other,
                &bind,
                Expect::Unavailable("bind-privileged-ports", "other"),
            ),
            (HostOs::Linux, &raw, Expect::Ok),
            (
                HostOs::MacOs,
                &raw,
                Expect::Unavailable("raw-network", "macos"),
            ),
            (
                HostOs::Windows,
                &raw,
                Expect::Unavailable("raw-network", "windows"),
            ),
            (
                HostOs::Other,
                &raw,
                Expect::Unavailable("raw-network", "other"),
            ),
            (HostOs::Linux, &privileged, Expect::Ok),
            (HostOs::MacOs, &privileged, Expect::Ok),
            (HostOs::Windows, &privileged, Expect::Ok),
            (HostOs::Other, &privileged, Expect::Unsupported),
        ];

        for (os, identity, expect) in cases {
            match expect {
                Expect::Ok => {
                    validate_identity(identity, os)
                        .unwrap_or_else(|err| panic!("{os:?} {identity:?} should pass: {err}"));
                }
                Expect::Unavailable(grant, platform) => {
                    let err = match validate_identity(identity, os) {
                        Ok(_) => panic!("{os:?} {identity:?} should fail"),
                        Err(err) => err.to_string(),
                    };
                    assert!(err.starts_with("identity:"), "{os:?}: {err}");
                    assert!(err.contains(grant), "{os:?}: {err}");
                    assert!(err.contains(platform), "{os:?}: {err}");
                }
                Expect::Unsupported => {
                    let err = validate_identity(identity, os)
                        .expect_err("privileged on Other should fail")
                        .to_string();
                    assert!(err.starts_with("identity:"), "{err}");
                    assert!(err.contains("run_as privileged"), "{err}");
                    assert!(err.contains("other"), "{err}");
                }
            }
        }
    }
}
