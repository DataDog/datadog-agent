//! Process Spawn Service
//!
//! Domain service for spawning new process instances from templates.
//! Used for socket activation with Accept=yes (one instance per connection)
//! and for manual instance spawning.

use crate::domain::ports::ProcessRepository;
use crate::domain::services::{ProcessLifecycleService, ProcessSupervisionService};
use crate::domain::{DomainError, ProcessId};
use std::sync::Arc;
use tracing::{debug, info};

/// Result of spawning a process instance
#[derive(Debug, Clone)]
pub struct SpawnedInstance {
    /// ID of the newly created instance
    pub id: ProcessId,
    /// Name of the newly created instance
    pub name: String,
    /// PID of the running instance
    pub pid: u32,
}

/// Process Spawn Service
///
/// Domain service that encapsulates the logic for spawning new process instances
/// from templates. This is domain logic, not application orchestration.
pub struct ProcessSpawningService {
    repository: Arc<dyn ProcessRepository>,
    lifecycle_service: Arc<ProcessLifecycleService>,
    _supervisor: Arc<ProcessSupervisionService>, // Kept for future use (e.g., direct registration)
}

impl ProcessSpawningService {
    /// Create a new process spawn service
    pub fn new(
        repository: Arc<dyn ProcessRepository>,
        lifecycle_service: Arc<ProcessLifecycleService>,
        supervisor: Arc<ProcessSupervisionService>,
    ) -> Self {
        Self {
            repository,
            lifecycle_service,
            _supervisor: supervisor,
        }
    }

    /// Spawn a new process instance from a template
    ///
    /// This creates a new process by cloning a template process with a unique name,
    /// then starting it with optional socket file descriptors.
    ///
    /// # Arguments
    /// * `template_name` - Name of the template process to clone
    /// * `socket_fds` - Socket file descriptors to pass to the new instance
    /// * `name_suffix` - Optional custom suffix for the instance name (uses UUID if None)
    ///
    /// # Returns
    /// * `Ok(SpawnedInstance)` - Information about the spawned instance
    /// * `Err(DomainError)` - If template not found, cloning fails, or start fails
    ///
    /// # Business Rules
    /// - Template process must exist
    /// - Template process must not be running (enforced by clone_with_name)
    /// - New instance gets unique name: `template-suffix` or `template-uuid`
    /// - Instance is saved before and after starting (state persistence)
    pub async fn spawn_from_template(
        &self,
        template_name: &str,
        socket_fds: Vec<i32>,
        name_suffix: Option<String>,
    ) -> Result<SpawnedInstance, DomainError> {
        debug!(
            template = %template_name,
            num_fds = socket_fds.len(),
            "Spawning new process instance from template"
        );

        // 1. Get template process from repository
        let template = self
            .repository
            .find_by_name(template_name)
            .await?
            .ok_or_else(|| {
                DomainError::ProcessNotFound(format!(
                    "Template process '{}' not found",
                    template_name
                ))
            })?;

        // 2. Generate unique name for the instance
        let unique_name = if let Some(suffix) = name_suffix {
            format!("{}-{}", template_name, suffix)
        } else {
            format!("{}-{}", template_name, uuid::Uuid::new_v4().as_simple())
        };

        info!(
            template = %template_name,
            instance = %unique_name,
            "Cloning template process to create new instance"
        );

        // 3. Clone the template process with the new name (domain logic in entity)
        let instance = template.clone_with_name(unique_name)?;

        // 4. Save the new instance to repository
        self.repository.save(instance.clone()).await?;

        debug!(
            instance = %instance.name(),
            instance_id = %instance.id(),
            "New process instance created and saved"
        );

        // 5. Start the instance with socket FDs using lifecycle service
        // Note: fd_env_var_names is empty for Accept=yes mode (uses LISTEN_FDS)
        let instance_id = instance.id();
        self.lifecycle_service
            .spawn_and_register(&instance_id, socket_fds, vec![], 0)
            .await?;

        // 6. Reload to get updated state (now running with PID)
        let instance = self
            .repository
            .find_by_id(&instance_id)
            .await?
            .ok_or_else(|| DomainError::ProcessNotFound(instance_id.to_string()))?;

        let pid = instance.pid().ok_or_else(|| {
            DomainError::InvalidCommand("Process started but has no PID".to_string())
        })?;

        info!(
            template = %template_name,
            instance = %instance.name(),
            instance_id = %instance.id(),
            pid = pid,
            "Process instance spawned and started successfully"
        );

        Ok(SpawnedInstance {
            id: instance.id(),
            name: instance.name().to_string(),
            pid,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::entities::Process;
    use crate::domain::RestartPolicy;

    #[test]
    fn test_spawned_instance_structure() {
        // Just verify the struct compiles and can be created
        let id = ProcessId::generate();
        let instance = SpawnedInstance {
            id,
            name: "test-instance".to_string(),
            pid: 1234,
        };

        assert_eq!(instance.name, "test-instance");
        assert_eq!(instance.pid, 1234);
    }

    #[test]
    fn test_clone_with_name_integration() {
        // Verify that Process::clone_with_name works as expected
        let mut template = Process::builder("web".to_string(), "/usr/bin/nginx".to_string())
            .build()
            .unwrap();
        template.set_restart_policy(RestartPolicy::Always);

        let mut env = std::collections::HashMap::new();
        env.insert("PORT".to_string(), "8080".to_string());
        template.set_env(env);

        let cloned = template
            .clone_with_name("web-instance-1".to_string())
            .unwrap();

        assert_eq!(cloned.name(), "web-instance-1");
        assert_ne!(cloned.id(), template.id());
        assert_eq!(cloned.command(), template.command());
        assert_eq!(cloned.restart_policy(), template.restart_policy());
        assert_eq!(cloned.env().get("PORT"), Some(&"8080".to_string()));
    }
}
