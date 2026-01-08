//! Dependency management service
//! Handles dependency validation, ordering, and conflict detection

use crate::domain::{DomainError, Process, ProcessState};
use std::collections::{HashMap, VecDeque};

pub struct DependencyResolutionService;

impl DependencyResolutionService {
    /// Validate that all dependencies exist in the process registry
    pub fn validate_dependencies(
        process: &Process,
        all_processes: &HashMap<String, Process>,
    ) -> Result<(), DomainError> {
        // Check requires
        for dep in process.requires() {
            if !all_processes.contains_key(dep) {
                return Err(DomainError::DependencyNotFound(dep.clone()));
            }
        }

        // Check binds_to
        for dep in process.binds_to() {
            if !all_processes.contains_key(dep) {
                return Err(DomainError::DependencyNotFound(dep.clone()));
            }
        }

        // Check wants (soft dependency - warn but don't fail)
        for dep in process.wants() {
            if !all_processes.contains_key(dep) {
                tracing::warn!(
                    process = %process.name(),
                    dependency = %dep,
                    "Soft dependency (wants) not found"
                );
            }
        }

        // Check after
        for dep in process.after() {
            if !all_processes.contains_key(dep) {
                tracing::warn!(
                    process = %process.name(),
                    dependency = %dep,
                    "Ordering dependency (after) not found"
                );
            }
        }

        // Check before
        for dep in process.before() {
            if !all_processes.contains_key(dep) {
                tracing::warn!(
                    process = %process.name(),
                    dependency = %dep,
                    "Ordering dependency (before) not found"
                );
            }
        }

        Ok(())
    }

    /// Check if starting a process would violate conflict constraints
    pub fn check_conflicts(
        process: &Process,
        all_processes: &HashMap<String, Process>,
    ) -> Result<(), DomainError> {
        for conflict in process.conflicts() {
            if let Some(conflicting_process) = all_processes.get(conflict) {
                if conflicting_process.state() == ProcessState::Running {
                    return Err(DomainError::ConflictingProcess {
                        process: process.name().to_string(),
                        conflicting_with: conflict.clone(),
                    });
                }
            }
        }
        Ok(())
    }

    /// Check if all hard dependencies (requires) are running
    pub fn check_required_dependencies(
        process: &Process,
        all_processes: &HashMap<String, Process>,
    ) -> Result<(), DomainError> {
        for dep in process.requires() {
            if let Some(dep_process) = all_processes.get(dep) {
                if dep_process.state() != ProcessState::Running {
                    return Err(DomainError::DependencyNotRunning {
                        process: process.name().to_string(),
                        dependency: dep.clone(),
                    });
                }
            } else {
                return Err(DomainError::DependencyNotFound(dep.clone()));
            }
        }
        Ok(())
    }

    /// Compute start order for a set of processes based on dependencies
    /// Returns an ordered list of process names (topological sort)
    pub fn compute_start_order(
        processes: &HashMap<String, Process>,
    ) -> Result<Vec<String>, DomainError> {
        let mut in_degree: HashMap<String, usize> = HashMap::new();
        let mut graph: HashMap<String, Vec<String>> = HashMap::new();

        // Initialize
        for name in processes.keys() {
            in_degree.insert(name.clone(), 0);
            graph.insert(name.clone(), Vec::new());
        }

        // Build dependency graph
        for (name, process) in processes {
            // "requires" creates an edge from dependency to dependent
            for dep in process.requires() {
                if processes.contains_key(dep) {
                    graph.get_mut(dep).unwrap().push(name.clone());
                    *in_degree.get_mut(name).unwrap() += 1;
                }
            }

            // "binds_to" creates an edge from dependency to dependent
            for dep in process.binds_to() {
                if processes.contains_key(dep) {
                    graph.get_mut(dep).unwrap().push(name.clone());
                    *in_degree.get_mut(name).unwrap() += 1;
                }
            }

            // "after" creates an edge from dependency to dependent
            for dep in process.after() {
                if processes.contains_key(dep) {
                    graph.get_mut(dep).unwrap().push(name.clone());
                    *in_degree.get_mut(name).unwrap() += 1;
                }
            }

            // "before" creates reverse edge (dependent to dependency)
            for dep in process.before() {
                if processes.contains_key(dep) {
                    graph.get_mut(name).unwrap().push(dep.clone());
                    *in_degree.get_mut(dep).unwrap() += 1;
                }
            }
        }

        // Kahn's algorithm for topological sort
        let mut queue: VecDeque<String> = VecDeque::new();
        let mut result = Vec::new();

        // Start with nodes that have no dependencies
        for (name, &degree) in &in_degree {
            if degree == 0 {
                queue.push_back(name.clone());
            }
        }

        while let Some(name) = queue.pop_front() {
            result.push(name.clone());

            // Remove edges
            if let Some(dependents) = graph.get(&name) {
                for dependent in dependents {
                    let degree = in_degree.get_mut(dependent).unwrap();
                    *degree -= 1;
                    if *degree == 0 {
                        queue.push_back(dependent.clone());
                    }
                }
            }
        }

        // Check for cycles
        if result.len() != processes.len() {
            return Err(DomainError::CircularDependency);
        }

        Ok(result)
    }

    /// Get all processes that should stop when a given process stops (binds_to relationship)
    pub fn get_bound_processes(
        stopping_process: &str,
        all_processes: &HashMap<String, Process>,
    ) -> Vec<String> {
        all_processes
            .iter()
            .filter(|(_, process)| process.binds_to().contains(&stopping_process.to_string()))
            .map(|(name, _)| name.clone())
            .collect()
    }

    /// Detect circular dependencies in a set of processes
    pub fn detect_cycles(processes: &HashMap<String, Process>) -> bool {
        Self::compute_start_order(processes).is_err()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_process(name: &str, command: &str) -> Process {
        Process::builder(name.to_string(), command.to_string())
            .build()
            .unwrap()
    }

    #[test]
    fn test_validate_dependencies_success() {
        let process = Process::builder("app".to_string(), "/bin/app".to_string())
            .requires(vec!["db".to_string()])
            .build()
            .unwrap();

        let mut processes = HashMap::new();
        processes.insert("app".to_string(), process.clone());
        processes.insert("db".to_string(), create_test_process("db", "/bin/db"));

        assert!(DependencyResolutionService::validate_dependencies(&process, &processes).is_ok());
    }

    #[test]
    fn test_validate_dependencies_missing() {
        let process = Process::builder("app".to_string(), "/bin/app".to_string())
            .requires(vec!["missing".to_string()])
            .build()
            .unwrap();

        let mut processes = HashMap::new();
        processes.insert("app".to_string(), process.clone());

        assert!(DependencyResolutionService::validate_dependencies(&process, &processes).is_err());
    }

    #[test]
    fn test_check_conflicts() {
        let process1 = Process::builder("nginx".to_string(), "/bin/nginx".to_string())
            .conflicts(vec!["apache".to_string()])
            .build()
            .unwrap();
        let mut process2 = create_test_process("apache", "/bin/apache");

        let mut processes = HashMap::new();
        processes.insert("nginx".to_string(), process1.clone());
        processes.insert("apache".to_string(), process2.clone());

        // Both created - no conflict yet
        assert!(DependencyResolutionService::check_conflicts(&process1, &processes).is_ok());

        // Mark apache as running
        process2.mark_starting().unwrap();
        process2.mark_running(123).unwrap();
        processes.insert("apache".to_string(), process2);

        // Now should conflict
        assert!(DependencyResolutionService::check_conflicts(&process1, &processes).is_err());
    }

    #[test]
    fn test_compute_start_order_linear() {
        let app = Process::builder("app".to_string(), "/bin/app".to_string())
            .requires(vec!["cache".to_string()])
            .build()
            .unwrap();
        let cache = Process::builder("cache".to_string(), "/bin/cache".to_string())
            .requires(vec!["db".to_string()])
            .build()
            .unwrap();
        let db = create_test_process("db", "/bin/db");

        let mut processes = HashMap::new();
        processes.insert("app".to_string(), app);
        processes.insert("cache".to_string(), cache);
        processes.insert("db".to_string(), db);

        let order = DependencyResolutionService::compute_start_order(&processes).unwrap();

        // db should come before cache, cache should come before app
        let db_idx = order.iter().position(|x| x == "db").unwrap();
        let cache_idx = order.iter().position(|x| x == "cache").unwrap();
        let app_idx = order.iter().position(|x| x == "app").unwrap();

        assert!(db_idx < cache_idx);
        assert!(cache_idx < app_idx);
    }

    #[test]
    fn test_compute_start_order_parallel() {
        let app = Process::builder("app".to_string(), "/bin/app".to_string())
            .requires(vec!["db".to_string()])
            .build()
            .unwrap();
        let worker1 = create_test_process("worker1", "/bin/worker");
        let worker2 = create_test_process("worker2", "/bin/worker");
        let db = create_test_process("db", "/bin/db");

        let mut processes = HashMap::new();
        processes.insert("app".to_string(), app);
        processes.insert("worker1".to_string(), worker1);
        processes.insert("worker2".to_string(), worker2);
        processes.insert("db".to_string(), db);

        let order = DependencyResolutionService::compute_start_order(&processes).unwrap();

        // db should come before app
        let db_idx = order.iter().position(|x| x == "db").unwrap();
        let app_idx = order.iter().position(|x| x == "app").unwrap();
        assert!(db_idx < app_idx);

        // workers can be anywhere (no dependencies)
        assert_eq!(order.len(), 4);
    }

    #[test]
    fn test_get_bound_processes() {
        let app = Process::builder("app".to_string(), "/bin/app".to_string())
            .binds_to(vec!["db".to_string()])
            .build()
            .unwrap();
        let worker = Process::builder("worker".to_string(), "/bin/worker".to_string())
            .binds_to(vec!["db".to_string()])
            .build()
            .unwrap();
        let db = create_test_process("db", "/bin/db");

        let mut processes = HashMap::new();
        processes.insert("app".to_string(), app);
        processes.insert("worker".to_string(), worker);
        processes.insert("db".to_string(), db);

        let bound = DependencyResolutionService::get_bound_processes("db", &processes);

        assert_eq!(bound.len(), 2);
        assert!(bound.contains(&"app".to_string()));
        assert!(bound.contains(&"worker".to_string()));
    }
}
