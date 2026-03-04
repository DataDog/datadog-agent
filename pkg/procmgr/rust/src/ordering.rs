// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::NamedProcess;
use log::warn;
use std::collections::{HashMap, HashSet};

/// Resolve the startup order of processes using a topological sort on the
/// dependency graph built from `after` and `before` fields.
///
/// Returns the indices into `configs` in the order they should be started.
/// If there is a cycle, returns an error listing the involved processes.
///
/// Among processes whose dependencies are equally satisfied, ties are broken
/// alphabetically by name (globally, not per batch).
pub fn resolve_order(configs: &[NamedProcess]) -> Result<Vec<usize>, CycleError> {
    let name_to_idx: HashMap<&str, usize> = configs
        .iter()
        .enumerate()
        .map(|(i, (name, _))| (name.as_str(), i))
        .collect();
    let n = configs.len();

    // adjacency: edges[a] contains b means "a must start before b"
    let mut edges: Vec<HashSet<usize>> = vec![HashSet::new(); n];
    let mut in_degree: Vec<u32> = vec![0; n];

    for (idx, (name, cfg)) in configs.iter().enumerate() {
        for dep in &cfg.after {
            match name_to_idx.get(dep.as_str()) {
                Some(&dep_idx) => {
                    if edges[dep_idx].insert(idx) {
                        in_degree[idx] += 1;
                    }
                }
                None => {
                    warn!("[{name}] after dependency '{dep}' not found, ignoring");
                }
            }
        }

        for dep in &cfg.before {
            match name_to_idx.get(dep.as_str()) {
                Some(&dep_idx) => {
                    if edges[idx].insert(dep_idx) {
                        in_degree[dep_idx] += 1;
                    }
                }
                None => {
                    warn!("[{name}] before dependency '{dep}' not found, ignoring");
                }
            }
        }
    }

    // Kahn's algorithm: keep ready set sorted in reverse so pop() yields the
    // alphabetically smallest name. Re-sort after inserting newly-ready nodes.
    let mut ready: Vec<usize> = (0..n).filter(|&i| in_degree[i] == 0).collect();
    ready.sort_by(|a, b| configs[*b].0.cmp(&configs[*a].0));

    let mut order: Vec<usize> = Vec::with_capacity(n);
    while let Some(idx) = ready.pop() {
        order.push(idx);

        for &dep_idx in &edges[idx] {
            in_degree[dep_idx] -= 1;
            if in_degree[dep_idx] == 0 {
                ready.push(dep_idx);
            }
        }
        ready.sort_by(|a, b| configs[*b].0.cmp(&configs[*a].0));
    }

    if order.len() != n {
        let cycle_members: Vec<String> = (0..n)
            .filter(|i| in_degree[*i] > 0)
            .map(|i| configs[i].0.clone())
            .collect();
        return Err(CycleError {
            processes: cycle_members,
        });
    }

    Ok(order)
}

#[derive(Debug)]
pub struct CycleError {
    pub processes: Vec<String>,
}

impl std::fmt::Display for CycleError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "dependency cycle detected among processes: {}",
            self.processes.join(", ")
        )
    }
}

impl std::error::Error for CycleError {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;

    fn cfg(after: &[&str], before: &[&str]) -> ProcessConfig {
        ProcessConfig {
            command: "/bin/true".to_string(),
            after: after.iter().map(String::from).collect(),
            before: before.iter().map(String::from).collect(),
            ..Default::default()
        }
    }

    fn names_in_order(configs: &[NamedProcess], order: &[usize]) -> Vec<String> {
        order.iter().map(|&i| configs[i].0.clone()).collect()
    }

    #[test]
    fn test_no_constraints_alphabetical() {
        let configs = vec![
            ("charlie".to_string(), cfg(&[], &[])),
            ("alpha".to_string(), cfg(&[], &[])),
            ("bravo".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(
            names_in_order(&configs, &order),
            vec!["alpha", "bravo", "charlie"]
        );
    }

    #[test]
    fn test_after_constraint() {
        let configs = vec![
            ("api".to_string(), cfg(&["db"], &[])),
            ("db".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["db", "api"]);
    }

    #[test]
    fn test_before_constraint() {
        let configs = vec![
            ("db".to_string(), cfg(&[], &["api"])),
            ("api".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["db", "api"]);
    }

    #[test]
    fn test_chain_a_before_b_before_c() {
        let configs = vec![
            ("c".to_string(), cfg(&["b"], &[])),
            ("a".to_string(), cfg(&[], &["b"])),
            ("b".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["a", "b", "c"]);
    }

    #[test]
    fn test_cycle_detected() {
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&["a"], &[])),
        ];
        let err = resolve_order(&configs).unwrap_err();
        assert!(err.processes.contains(&"a".to_string()));
        assert!(err.processes.contains(&"b".to_string()));
    }

    #[test]
    fn test_missing_dependency_ignored() {
        let configs = vec![
            ("api".to_string(), cfg(&["nonexistent"], &[])),
            ("db".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(order.len(), 2);
    }

    #[test]
    fn test_diamond_dependency() {
        // a -> b, a -> c, b -> d, c -> d
        let configs = vec![
            ("a".to_string(), cfg(&[], &["b", "c"])),
            ("b".to_string(), cfg(&[], &["d"])),
            ("c".to_string(), cfg(&[], &["d"])),
            ("d".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        let names = names_in_order(&configs, &order);
        assert_eq!(names, vec!["a", "b", "c", "d"]);
    }

    #[test]
    fn test_single_process() {
        let configs = vec![("solo".to_string(), cfg(&[], &[]))];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(order, vec![0]);
    }

    #[test]
    fn test_empty() {
        let configs: Vec<NamedProcess> = vec![];
        let order = resolve_order(&configs).unwrap();
        assert!(order.is_empty());
    }

    #[test]
    fn test_alphabetical_tiebreak_with_constraints() {
        // d depends on a; b and c are unconstrained
        // expected: a, b, c, d (a first because d depends on it; b,c alphabetical; d last)
        let configs = vec![
            ("d".to_string(), cfg(&["a"], &[])),
            ("c".to_string(), cfg(&[], &[])),
            ("b".to_string(), cfg(&[], &[])),
            ("a".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        let names = names_in_order(&configs, &order);
        assert_eq!(names, vec!["a", "b", "c", "d"]);
    }

    #[test]
    fn test_global_tiebreak_newly_ready_before_queued() {
        // b -> a (b must start before a), c is unconstrained
        // After b is popped, a becomes ready. a < c alphabetically, so a should come before c.
        // expected: b, a, c (not b, c, a)
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&[], &[])),
            ("c".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["b", "a", "c"]);
    }

    #[test]
    fn test_self_dependency_is_cycle() {
        let configs = vec![("a".to_string(), cfg(&["a"], &[]))];
        let err = resolve_order(&configs).unwrap_err();
        assert_eq!(err.processes, vec!["a"]);
    }

    #[test]
    fn test_duplicate_dependency_in_list() {
        // Listing "b" twice in after should not double-count in_degree.
        let configs = vec![
            ("a".to_string(), cfg(&["b", "b"], &[])),
            ("b".to_string(), cfg(&[], &[])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["b", "a"]);
    }

    #[test]
    fn test_redundant_after_and_before_same_edge() {
        // A says after:["B"] and B says before:["A"] — both express B→A.
        // Should produce exactly one edge, not a double-count.
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&[], &["a"])),
        ];
        let order = resolve_order(&configs).unwrap();
        assert_eq!(names_in_order(&configs, &order), vec!["b", "a"]);
    }
}
