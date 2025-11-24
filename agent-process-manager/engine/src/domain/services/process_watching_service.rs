//! Process Watcher
//! Event-driven process exit monitoring using tokio tasks
//! Inspired by the old architecture's HealthMonitoringManager.start_exit_monitor

use crate::domain::{ports::ProcessExitHandle, Process, ProcessId};
use tokio::sync::mpsc;
use tracing::{debug, error, info, warn};

/// Event sent when a process exits
#[derive(Debug, Clone)]
pub struct ProcessExitEvent {
    pub process_id: ProcessId,
    pub pid: u32,
    pub exit_code: i32,
}

/// Process watcher that monitors process exits and sends notifications
pub struct ProcessWatchingService {
    /// Channel for sending exit events
    exit_tx: mpsc::UnboundedSender<ProcessExitEvent>,
}

impl ProcessWatchingService {
    /// Create a new ProcessWatchingService
    /// Returns the watcher and a receiver for exit events
    pub fn new() -> (Self, mpsc::UnboundedReceiver<ProcessExitEvent>) {
        let (exit_tx, exit_rx) = mpsc::unbounded_channel();
        (Self { exit_tx }, exit_rx)
    }

    /// Start monitoring a process for exit
    /// Spawns a background task that awaits the exit_handle and sends a notification
    pub fn watch_process(&self, process: &Process, exit_handle: ProcessExitHandle) {
        let process_id = process.id();
        let pid = process.pid().unwrap_or(0);
        let process_name = process.name().to_string();
        let exit_tx = self.exit_tx.clone();

        tokio::spawn(async move {
            debug!(
                process_id = %process_id,
                pid = pid,
                name = %process_name,
                "Monitoring process for exit"
            );

            // Await the exit handle (event-driven, no polling!)
            match exit_handle.await {
                Ok(exit_code) => {
                    info!(
                        process_id = %process_id,
                        pid = pid,
                        name = %process_name,
                        exit_code = exit_code,
                        "Process exited"
                    );

                    // Send exit event
                    if let Err(e) = exit_tx.send(ProcessExitEvent {
                        process_id,
                        pid,
                        exit_code,
                    }) {
                        error!(
                            process_id = %process_id,
                            error = %e,
                            "Failed to send exit event (channel closed)"
                        );
                    }
                }
                Err(e) => {
                    warn!(
                        process_id = %process_id,
                        pid = pid,
                        name = %process_name,
                        error = %e,
                        "Error waiting for process exit, treating as failure"
                    );

                    // Send exit event with failure code
                    let _ = exit_tx.send(ProcessExitEvent {
                        process_id,
                        pid,
                        exit_code: 1,
                    });
                }
            }
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::Process;

    #[tokio::test]
    async fn test_watcher_receives_exit_event() {
        let (watcher, mut exit_rx) = ProcessWatchingService::new();

        let process = Process::builder("test".to_string(), "/bin/test".to_string())
            .build()
            .unwrap();
        let process_id = process.id();

        // Create a fake exit handle that immediately resolves
        let exit_handle = Box::pin(async { Ok(42) });

        watcher.watch_process(&process, exit_handle);

        // Wait for the exit event
        let event = tokio::time::timeout(std::time::Duration::from_secs(1), exit_rx.recv())
            .await
            .expect("Timeout waiting for exit event")
            .expect("Channel closed");

        assert_eq!(event.process_id, process_id);
        assert_eq!(event.exit_code, 42);
    }

    #[tokio::test]
    async fn test_watcher_handles_multiple_processes() {
        let (watcher, mut exit_rx) = ProcessWatchingService::new();

        let process1 = Process::builder("test1".to_string(), "/bin/test1".to_string())
            .build()
            .unwrap();
        let process2 = Process::builder("test2".to_string(), "/bin/test2".to_string())
            .build()
            .unwrap();
        let id1 = process1.id();
        let id2 = process2.id();

        // Create fake exit handles
        let exit_handle1 = Box::pin(async { Ok(0) });
        let exit_handle2 = Box::pin(async { Ok(1) });

        watcher.watch_process(&process1, exit_handle1);
        watcher.watch_process(&process2, exit_handle2);

        // Collect both events
        let mut events = vec![];
        for _ in 0..2 {
            if let Ok(Some(event)) =
                tokio::time::timeout(std::time::Duration::from_secs(1), exit_rx.recv()).await
            {
                events.push(event);
            }
        }

        assert_eq!(events.len(), 2);
        assert!(events
            .iter()
            .any(|e| e.process_id == id1 && e.exit_code == 0));
        assert!(events
            .iter()
            .any(|e| e.process_id == id2 && e.exit_code == 1));
    }
}
