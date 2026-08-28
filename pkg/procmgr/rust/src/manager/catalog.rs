// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::find_index_by_name;
use crate::config::{self, ConfigLoader, ProcessConfig, ProcessDefinition};
use crate::ordering;
use crate::process::ManagedProcess;
use crate::shutdown;
use crate::uuid_gen::UuidGenerator;
use log::{debug, info, warn};
use std::sync::Arc;
use tokio::sync::RwLock;
use tonic::Status;

pub(in crate::manager) struct ProcessCatalog {
    processes: RwLock<Vec<ManagedProcess>>,
    startup_order: RwLock<Vec<usize>>,
    uuid_gen: Arc<dyn UuidGenerator>,
    config_source: String,
    config_location: String,
}

impl ProcessCatalog {
    pub fn load(config_loader: &dyn ConfigLoader, uuid_gen: Arc<dyn UuidGenerator>) -> Self {
        let config_source = config_loader.source().to_string();
        let config_location = config_loader.location();
        let processes: Vec<ManagedProcess> = config_loader
            .load()
            .into_iter()
            .map(|pd| ManagedProcess::new_config(pd.name, uuid_gen.generate(), pd.config))
            .collect();
        let startup_order = Self::recompute_startup_order(&processes).order;
        Self {
            processes: RwLock::new(processes),
            startup_order: RwLock::new(startup_order),
            uuid_gen,
            config_source,
            config_location,
        }
    }

    pub(crate) fn config_source(&self) -> &str {
        &self.config_source
    }

    pub(crate) fn config_location(&self) -> &str {
        &self.config_location
    }

    pub(in crate::manager) async fn read_processes(
        &self,
    ) -> tokio::sync::RwLockReadGuard<'_, Vec<ManagedProcess>> {
        self.processes.read().await
    }

    pub(in crate::manager) async fn write_processes(
        &self,
    ) -> tokio::sync::RwLockWriteGuard<'_, Vec<ManagedProcess>> {
        self.processes.write().await
    }

    pub(in crate::manager) async fn startup_order(
        &self,
    ) -> tokio::sync::RwLockReadGuard<'_, Vec<usize>> {
        self.startup_order.read().await
    }

    pub(in crate::manager) async fn append_runtime(
        &self,
        name: &str,
        config: config::ProcessConfig,
    ) -> Result<(String, usize, bool, Vec<String>), Status> {
        let mut procs = self.processes.write().await;
        let mut startup_order = self.startup_order.write().await;
        let (uuid, auto_start_idx, auto_start) =
            self.append_runtime_process(&mut procs, name, config)?;
        let warnings = Self::sync_startup_order(&procs, &mut startup_order);
        Ok((uuid, auto_start_idx, auto_start, warnings))
    }

    pub(in crate::manager) async fn shutdown(&self) {
        let order: Vec<usize> = self
            .startup_order
            .read()
            .await
            .iter()
            .copied()
            .rev()
            .collect();
        {
            let mut procs = self.processes.write().await;
            for &idx in &order {
                procs[idx].request_stop();
            }
        }
        let signal_time =
            crate::platform::service_stop_signal_time().unwrap_or_else(std::time::Instant::now);
        let budget = shutdown::ShutdownBudget::service_stop(signal_time);
        for &idx in &order {
            let mut procs = self.processes.write().await;
            procs[idx].wait_for_stop_since(budget).await;
        }
    }

    fn append_runtime_process(
        &self,
        procs: &mut Vec<ManagedProcess>,
        name: &str,
        config: ProcessConfig,
    ) -> Result<(String, usize, bool), Status> {
        if find_index_by_name(procs, name).is_some() {
            return Err(Status::already_exists(format!(
                "process '{name}' already exists"
            )));
        }
        let proc = ManagedProcess::new_runtime(name.to_string(), self.uuid_gen.generate(), config);
        let uuid = proc.uuid().to_owned();
        let auto_start = proc.may_auto_start();
        info!("[{name}] created via RPC (uuid={uuid})");
        procs.push(proc);
        Ok((uuid, procs.len() - 1, auto_start))
    }

    fn sync_startup_order(procs: &[ManagedProcess], startup_order: &mut Vec<usize>) -> Vec<String> {
        let result = Self::recompute_startup_order(procs);
        *startup_order = result.order;
        result.warnings
    }

    fn recompute_startup_order(procs: &[ManagedProcess]) -> StartupOrderResult {
        let defs: Vec<ProcessDefinition> = procs
            .iter()
            .map(|p| ProcessDefinition {
                name: p.name().to_string(),
                config: p.config().clone(),
            })
            .collect();
        let result = ordering::resolve_order(&defs);
        if !result.skipped.is_empty() {
            warn!(
                "dependency cycle detected, skipping processes: {}",
                result.skipped.join(", ")
            );
        }
        let names: Vec<&str> = result.order.iter().map(|&i| procs[i].name()).collect();
        debug!("startup order: {}", names.join(" -> "));
        StartupOrderResult {
            order: result.order,
            warnings: result.warnings,
        }
    }
}

struct StartupOrderResult {
    order: Vec<usize>,
    warnings: Vec<String>,
}

#[cfg(test)]
mod startup_order_tests {
    use super::*;
    use crate::config::{ProcessConfig, StaticConfigLoader};
    use crate::test_helpers;
    use crate::uuid_gen::V4UuidGenerator;
    use std::sync::Arc;

    fn named_config(name: &str) -> ManagedProcess {
        let (cmd, args) = test_helpers::true_cmd();
        ManagedProcess::new_config(
            name.to_string(),
            format!("uuid-{name}"),
            ProcessConfig {
                command: cmd.to_string(),
                args,
                ..Default::default()
            },
        )
    }

    #[test]
    fn sync_startup_order_matches_process_count() {
        let procs = vec![named_config("alpha"), named_config("bravo")];
        let mut order = vec![0];

        let _warnings = ProcessCatalog::sync_startup_order(&procs, &mut order);

        assert_eq!(order.len(), procs.len());
        assert_eq!(
            order
                .iter()
                .copied()
                .collect::<std::collections::HashSet<_>>()
                .len(),
            procs.len()
        );
    }

    #[test]
    fn stale_order_write_drops_process_from_shutdown_plan() {
        let procs = vec![named_config("alpha"), named_config("bravo")];
        let fresh = ProcessCatalog::recompute_startup_order(&procs).order;
        let stale = [0];

        assert_eq!(fresh.len(), 2);
        assert_eq!(stale.len(), 1);
        assert!(
            !stale.contains(&1),
            "stale order omits bravo and would skip it during shutdown"
        );
    }

    #[test]
    fn load_builds_matching_startup_order() {
        let catalog = ProcessCatalog::load(
            &StaticConfigLoader::new(vec![
                ProcessDefinition {
                    name: "alpha".to_string(),
                    config: ProcessConfig::default(),
                },
                ProcessDefinition {
                    name: "bravo".to_string(),
                    config: ProcessConfig::default(),
                },
            ]),
            Arc::new(V4UuidGenerator),
        );

        let rt = tokio::runtime::Runtime::new().unwrap();
        let order = rt.block_on(catalog.startup_order());
        assert_eq!(order.len(), 2);
    }
}
