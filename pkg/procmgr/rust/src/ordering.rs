// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::config::NamedProcess;
use log::warn;
use std::collections::{HashMap, HashSet};

/// Result of resolving startup order via topological sort.
pub struct ResolvedOrder {
    /// Indices into the original configs slice, in startup order.
    /// Processes involved in a cycle are excluded.
    pub order: Vec<usize>,
    /// Names of processes that were skipped due to dependency cycles.
    pub skipped: Vec<String>,
}

/// Resolve the startup order of processes using a topological sort on the
/// dependency graph built from `after` and `before` fields.
///
/// Processes involved in a dependency cycle are excluded from the order and
/// reported in `skipped` so the caller can log them without stopping the
/// daemon. Non-cyclic processes are always started in the correct order.
///
/// Among processes whose dependencies are equally satisfied, ties are broken
/// alphabetically by name (globally, not per batch).
pub fn resolve_order(configs: &[NamedProcess]) -> ResolvedOrder {
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

    let skipped: Vec<String> = (0..n)
        .filter(|i| in_degree[*i] > 0)
        .map(|i| configs[i].0.clone())
        .collect();

    ResolvedOrder { order, skipped }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ProcessConfig;

    fn cfg(after: &[&str], before: &[&str]) -> ProcessConfig {
        ProcessConfig {
            command: "/bin/true".to_string(),
            after: after.iter().copied().map(String::from).collect(),
            before: before.iter().copied().map(String::from).collect(),
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
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(
            names_in_order(&configs, &result.order),
            vec!["alpha", "bravo", "charlie"]
        );
    }

    #[test]
    fn test_after_constraint() {
        let configs = vec![
            ("api".to_string(), cfg(&["db"], &[])),
            ("db".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["db", "api"]);
    }

    #[test]
    fn test_before_constraint() {
        let configs = vec![
            ("db".to_string(), cfg(&[], &["api"])),
            ("api".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["db", "api"]);
    }

    #[test]
    fn test_chain_a_before_b_before_c() {
        let configs = vec![
            ("c".to_string(), cfg(&["b"], &[])),
            ("a".to_string(), cfg(&[], &["b"])),
            ("b".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["a", "b", "c"]);
    }

    #[test]
    fn test_cycle_skips_involved_processes() {
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&["a"], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.order.is_empty());
        assert!(result.skipped.contains(&"a".to_string()));
        assert!(result.skipped.contains(&"b".to_string()));
    }

    #[test]
    fn test_cycle_starts_non_cyclic_processes() {
        // a <-> b form a cycle; c and d are independent
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&["a"], &[])),
            ("c".to_string(), cfg(&[], &[])),
            ("d".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert_eq!(names_in_order(&configs, &result.order), vec!["c", "d"]);
        assert_eq!(result.skipped, vec!["a", "b"]);
    }

    #[test]
    fn test_missing_dependency_ignored() {
        let configs = vec![
            ("api".to_string(), cfg(&["nonexistent"], &[])),
            ("db".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(result.order.len(), 2);
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
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(
            names_in_order(&configs, &result.order),
            vec!["a", "b", "c", "d"]
        );
    }

    #[test]
    fn test_single_process() {
        let configs = vec![("solo".to_string(), cfg(&[], &[]))];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["solo"]);
    }

    #[test]
    fn test_empty() {
        let configs: Vec<NamedProcess> = vec![];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert!(result.order.is_empty());
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
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(
            names_in_order(&configs, &result.order),
            vec!["a", "b", "c", "d"]
        );
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
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["b", "a", "c"]);
    }

    #[test]
    fn test_self_dependency_is_cycle() {
        let configs = vec![("a".to_string(), cfg(&["a"], &[]))];
        let result = resolve_order(&configs);
        assert!(result.order.is_empty());
        assert_eq!(result.skipped, vec!["a"]);
    }

    #[test]
    fn test_duplicate_dependency_in_list() {
        // Listing "b" twice in after should not double-count in_degree.
        let configs = vec![
            ("a".to_string(), cfg(&["b", "b"], &[])),
            ("b".to_string(), cfg(&[], &[])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["b", "a"]);
    }

    #[test]
    fn test_redundant_after_and_before_same_edge() {
        // A says after:["B"] and B says before:["A"] — both express B→A.
        // Should produce exactly one edge, not a double-count.
        let configs = vec![
            ("a".to_string(), cfg(&["b"], &[])),
            ("b".to_string(), cfg(&[], &["a"])),
        ];
        let result = resolve_order(&configs);
        assert!(result.skipped.is_empty());
        assert_eq!(names_in_order(&configs, &result.order), vec!["b", "a"]);
    }
}
