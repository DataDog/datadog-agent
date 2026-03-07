// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub mod server;
pub mod service;

#[cfg(not(bazel))]
pub mod proto {
    tonic::include_proto!("datadog.procmgr");

    pub const FILE_DESCRIPTOR_SET: &[u8] =
        tonic::include_file_descriptor_set!("process_manager_descriptor");
}

#[cfg(bazel)]
pub mod proto {
    pub use process_manager_proto::datadog::procmgr::*;
    pub const FILE_DESCRIPTOR_SET: &[u8] =
        process_manager_proto::datadog::procmgr::FILE_DESCRIPTOR_SET;
}

#[cfg(test)]
mod tests {
    use super::proto;
    use super::proto::process_manager_client::ProcessManagerClient;
    use super::service::ProcessManagerService;
    use crate::manager::ProcessManager;
    use crate::command::Command;
    use crate::config::{ProcessConfig, RestartPolicy};
    use crate::process::ManagedProcess;
    use hyper_util::rt::TokioIo;
    use tokio::net::UnixListener;
    use tokio::sync::mpsc;
    use tokio_stream::wrappers::UnixListenerStream;
    use tonic::transport::{Channel, Endpoint, Uri};
    use tower::service_fn;

    async fn start_test_server(
        processes: Vec<ManagedProcess>,
    ) -> (
        ProcessManagerClient<Channel>,
        tokio::sync::oneshot::Sender<()>,
    ) {
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let dir = tempfile::tempdir().unwrap();
        let sock_path = dir.path().join("test.sock");
        let uds = UnixListener::bind(&sock_path).unwrap();
        let uds_stream = UnixListenerStream::new(uds);

        let mgr = ProcessManager::from_processes(processes);
        let svc = ProcessManagerService::new(mgr.clone(), "/test/config/path".to_string(), cmd_tx);

        let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();

        tokio::spawn(async move {
            let pm_service = proto::process_manager_server::ProcessManagerServer::new(svc);

            #[cfg(not(bazel))]
            let router = {
                let reflection = tonic_reflection::server::Builder::configure()
                    .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
                    .build_v1()
                    .unwrap();
                tonic::transport::Server::builder()
                    .add_service(reflection)
                    .add_service(pm_service)
            };

            #[cfg(bazel)]
            let router = tonic::transport::Server::builder().add_service(pm_service);

            router
                .serve_with_incoming_shutdown(uds_stream, async {
                    let _ = shutdown_rx.await;
                })
                .await
                .unwrap();
            drop(dir);
        });

        let (exit_tx, mut exit_rx) = mpsc::channel::<crate::manager::ExitEvent>(256);
        let mgr_loop = mgr.clone();
        let exit_tx_loop = exit_tx.clone();
        tokio::spawn(async move {
            loop {
                tokio::select! {
                    Some(cmd) = cmd_rx.recv() => {
                        match cmd {
                            Command::Create { name, config, reply } => {
                                let _ = reply.send(
                                    mgr_loop.handle_create(name, *config).await,
                                );
                            }
                            Command::Start { name, reply } => {
                                let _ = reply.send(
                                    mgr_loop.handle_start(&name, &exit_tx_loop).await,
                                );
                            }
                            Command::Stop { name, reply } => {
                                let _ = reply.send(mgr_loop.handle_stop(&name).await);
                            }
                            Command::ReloadConfig { reply } => {
                                let _ = reply.send(
                                    mgr_loop.handle_reload_config(&exit_tx_loop).await,
                                );
                            }
                        }
                    }
                    Some(event) = exit_rx.recv() => {
                        let restart_tx = mpsc::channel::<String>(256).0;
                        mgr_loop.handle_exit(event, &restart_tx).await;
                    }
                    else => break,
                }
            }
        });

        let channel = Endpoint::try_from("http://[::]:50051")
            .unwrap()
            .connect_with_connector(service_fn(move |_: Uri| {
                let path = sock_path.clone();
                async move {
                    tokio::net::UnixStream::connect(path)
                        .await
                        .map(TokioIo::new)
                }
            }))
            .await
            .unwrap();

        let client = ProcessManagerClient::new(channel);
        (client, shutdown_tx)
    }

    #[tokio::test]
    async fn test_list_returns_processes() {
        let procs = vec![
            ManagedProcess::new(
                "alpha".to_string(),
                ProcessConfig {
                    command: "/usr/bin/alpha".to_string(),
                    args: vec!["--flag".to_string()],
                    ..Default::default()
                },
            ),
            ManagedProcess::new(
                "beta".to_string(),
                ProcessConfig {
                    command: "/usr/bin/beta".to_string(),
                    ..Default::default()
                },
            ),
        ];

        let (mut client, _shutdown) = start_test_server(procs).await;
        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();

        assert_eq!(resp.processes.len(), 2);
        assert_eq!(resp.processes[0].name, "alpha");
        assert_eq!(resp.processes[0].command, "/usr/bin/alpha");
        assert_eq!(resp.processes[0].args, vec!["--flag"]);
        assert_eq!(resp.processes[0].state, proto::ProcessState::Created as i32);
        assert_eq!(resp.processes[1].name, "beta");
        assert_eq!(resp.processes[1].command, "/usr/bin/beta");
        assert_eq!(resp.processes[1].state, proto::ProcessState::Created as i32);
    }

    #[tokio::test]
    async fn test_describe_found() {
        let mut env = std::collections::HashMap::new();
        env.insert("FOO".to_string(), "bar".to_string());
        env.insert("BAZ".to_string(), "qux".to_string());

        let procs = vec![ManagedProcess::new(
            "my-service".to_string(),
            ProcessConfig {
                command: "/bin/myservice".to_string(),
                args: vec!["--port".to_string(), "8080".to_string()],
                description: Some("A test service".to_string()),
                working_dir: Some("/opt/service".to_string()),
                auto_start: false,
                env,
                restart: RestartPolicy::OnFailure,
                stdout: "/var/log/out.log".to_string(),
                stderr: "/var/log/err.log".to_string(),
                condition_path_exists: Some("/opt/bin/myservice".to_string()),
                after: vec!["dep-a".to_string()],
                before: vec!["dep-b".to_string()],
                ..Default::default()
            },
        )];

        let (mut client, _shutdown) = start_test_server(procs).await;
        let resp = client
            .describe(proto::DescribeRequest {
                name: "my-service".to_string(),
            })
            .await
            .unwrap()
            .into_inner();

        let detail = resp.detail.unwrap();
        assert_eq!(detail.name, "my-service");
        assert_eq!(detail.description, "A test service");
        assert_eq!(detail.command, "/bin/myservice");
        assert_eq!(detail.args, vec!["--port", "8080"]);
        assert_eq!(detail.working_dir, "/opt/service");
        assert!(!detail.auto_start);
        assert_eq!(detail.env.get("FOO").unwrap(), "bar");
        assert_eq!(detail.env.get("BAZ").unwrap(), "qux");
        assert_eq!(detail.env.len(), 2);
        assert_eq!(detail.restart_policy, "on-failure");
        assert_eq!(detail.stdout, "/var/log/out.log");
        assert_eq!(detail.stderr, "/var/log/err.log");
        assert_eq!(detail.condition_path_exists, "/opt/bin/myservice");
        assert_eq!(detail.after, vec!["dep-a"]);
        assert_eq!(detail.before, vec!["dep-b"]);
        assert_eq!(detail.state, proto::ProcessState::Created as i32);
        assert_eq!(detail.pid, 0);
    }

    #[tokio::test]
    async fn test_describe_not_found() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;
        let status = client
            .describe(proto::DescribeRequest {
                name: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();

        assert_eq!(status.code(), tonic::Code::NotFound);
        assert_eq!(status.message(), "process 'nonexistent' not found");
    }

    #[tokio::test]
    async fn test_get_status() {
        let procs = vec![
            ManagedProcess::new(
                "running-proc".to_string(),
                ProcessConfig {
                    command: "/bin/true".to_string(),
                    ..Default::default()
                },
            ),
            ManagedProcess::new(
                "another-proc".to_string(),
                ProcessConfig {
                    command: "/bin/false".to_string(),
                    ..Default::default()
                },
            ),
        ];

        let (mut client, _shutdown) = start_test_server(procs).await;
        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();

        assert!(resp.ready);
        assert_eq!(resp.version, env!("CARGO_PKG_VERSION"));
        assert_eq!(resp.total_processes, 2);
        assert_eq!(resp.running_processes, 0);
        assert_eq!(resp.created_processes, 2);
        assert_eq!(resp.exited_processes, 0);
        assert_eq!(resp.config_path, "/test/config/path");
    }

    #[tokio::test]
    async fn test_list_empty() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;
        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        assert!(resp.processes.is_empty());
    }

    #[tokio::test]
    async fn test_describe_picks_correct_process() {
        let procs = vec![
            ManagedProcess::new(
                "alpha".to_string(),
                ProcessConfig {
                    command: "/usr/bin/alpha".to_string(),
                    ..Default::default()
                },
            ),
            ManagedProcess::new(
                "beta".to_string(),
                ProcessConfig {
                    command: "/usr/bin/beta".to_string(),
                    description: Some("The beta service".to_string()),
                    ..Default::default()
                },
            ),
            ManagedProcess::new(
                "gamma".to_string(),
                ProcessConfig {
                    command: "/usr/bin/gamma".to_string(),
                    ..Default::default()
                },
            ),
        ];

        let (mut client, _shutdown) = start_test_server(procs).await;

        let resp = client
            .describe(proto::DescribeRequest {
                name: "beta".to_string(),
            })
            .await
            .unwrap()
            .into_inner();
        let detail = resp.detail.unwrap();
        assert_eq!(detail.name, "beta");
        assert_eq!(detail.command, "/usr/bin/beta");
        assert_eq!(detail.description, "The beta service");

        let resp = client
            .describe(proto::DescribeRequest {
                name: "gamma".to_string(),
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.detail.unwrap().name, "gamma");
    }

    #[tokio::test]
    async fn test_get_status_empty() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;
        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();

        assert!(resp.ready);
        assert_eq!(resp.total_processes, 0);
        assert_eq!(resp.running_processes, 0);
        assert_eq!(resp.stopped_processes, 0);
        assert_eq!(resp.failed_processes, 0);
        assert_eq!(resp.created_processes, 0);
        assert_eq!(resp.exited_processes, 0);
    }

    #[tokio::test]
    async fn test_get_status_mixed_states() {
        // Running: spawn a long-lived process
        let mut proc_running = ManagedProcess::new(
            "running-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );
        proc_running.spawn().unwrap();
        let mut child = proc_running.take_child().unwrap();

        // Failed: spawn a process that exits non-zero
        let mut proc_failed = ManagedProcess::new(
            "failed-svc".to_string(),
            ProcessConfig {
                command: "/bin/sh".to_string(),
                args: vec!["-c".to_string(), "exit 1".to_string()],
                ..Default::default()
            },
        );
        proc_failed.spawn().unwrap();
        let mut child_fail = proc_failed.take_child().unwrap();
        let status = child_fail.wait().await.unwrap();
        proc_failed.set_last_status(status);

        // Stopped: spawn then explicitly stop
        let mut proc_stopped = ManagedProcess::new(
            "stopped-svc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );
        proc_stopped.spawn().unwrap();
        proc_stopped.request_stop();
        proc_stopped.wait_for_stop().await;

        // Exited: spawn a process that exits cleanly
        let mut proc_exited = ManagedProcess::new(
            "exited-svc".to_string(),
            ProcessConfig {
                command: "/bin/sh".to_string(),
                args: vec!["-c".to_string(), "exit 0".to_string()],
                ..Default::default()
            },
        );
        proc_exited.spawn().unwrap();
        let mut child_exit = proc_exited.take_child().unwrap();
        let status_exit = child_exit.wait().await.unwrap();
        proc_exited.set_last_status(status_exit);

        // Created: never spawned
        let proc_created = ManagedProcess::new(
            "created-svc".to_string(),
            ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        );

        let procs = vec![
            proc_running,
            proc_failed,
            proc_stopped,
            proc_exited,
            proc_created,
        ];
        let (mut client, _shutdown) = start_test_server(procs).await;

        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();

        assert_eq!(resp.total_processes, 5);
        assert_eq!(resp.running_processes, 1);
        assert_eq!(resp.failed_processes, 1);
        assert_eq!(resp.stopped_processes, 1);
        assert_eq!(resp.exited_processes, 1);
        assert_eq!(resp.created_processes, 1);

        child.kill().await.ok();
    }

    #[tokio::test]
    async fn test_list_shows_running_pid() {
        let mut proc = ManagedProcess::new(
            "live-proc".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );
        proc.spawn().unwrap();
        let mut child = proc.take_child().unwrap();

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;
        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();

        assert_eq!(resp.processes.len(), 1);
        assert_eq!(resp.processes[0].name, "live-proc");
        assert_eq!(resp.processes[0].state, proto::ProcessState::Running as i32);
        assert!(
            resp.processes[0].pid > 0,
            "running process should report a non-zero pid"
        );

        child.kill().await.ok();
    }

    // -- Write RPC tests --

    #[tokio::test]
    async fn test_start_rpc_success() {
        let proc = ManagedProcess::new(
            "sleeper".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        client
            .start(proto::StartRequest {
                name: "sleeper".to_string(),
            })
            .await
            .unwrap();

        // Verify process is now running
        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.processes[0].state, proto::ProcessState::Running as i32);
        assert!(resp.processes[0].pid > 0);

        // Clean up
        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(resp.processes[0].pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_start_rpc_not_found() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let err = client
            .start(proto::StartRequest {
                name: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_start_rpc_already_running() {
        let mut proc = ManagedProcess::new(
            "running".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );
        proc.spawn().unwrap();
        let mut child = proc.take_child().unwrap();

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        let err = client
            .start(proto::StartRequest {
                name: "running".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::FailedPrecondition);

        child.kill().await.ok();
    }

    #[tokio::test]
    async fn test_stop_rpc_success() {
        let proc = ManagedProcess::new(
            "to-stop".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        // Start via RPC so the watcher is wired
        client
            .start(proto::StartRequest {
                name: "to-stop".to_string(),
            })
            .await
            .unwrap();

        client
            .stop(proto::StopRequest {
                name: "to-stop".to_string(),
            })
            .await
            .unwrap();

        // Give the watcher time to process the exit event
        tokio::time::sleep(std::time::Duration::from_millis(200)).await;

        let resp = client
            .describe(proto::DescribeRequest {
                name: "to-stop".to_string(),
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(
            resp.detail.unwrap().state,
            proto::ProcessState::Stopped as i32
        );
    }

    #[tokio::test]
    async fn test_stop_rpc_not_found() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let err = client
            .stop(proto::StopRequest {
                name: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_stop_rpc_not_running() {
        let proc = ManagedProcess::new(
            "idle".to_string(),
            ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        );

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        let err = client
            .stop(proto::StopRequest {
                name: "idle".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::FailedPrecondition);
    }

    #[tokio::test]
    async fn test_start_then_stop_round_trip() {
        let proc = ManagedProcess::new(
            "lifecycle".to_string(),
            ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        );

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        // Start
        client
            .start(proto::StartRequest {
                name: "lifecycle".to_string(),
            })
            .await
            .unwrap();

        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.running_processes, 1);

        // Stop
        client
            .stop(proto::StopRequest {
                name: "lifecycle".to_string(),
            })
            .await
            .unwrap();

        tokio::time::sleep(std::time::Duration::from_millis(200)).await;

        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.running_processes, 0);
        assert_eq!(resp.stopped_processes, 1);
    }

    // -- Create RPC tests --

    #[tokio::test]
    async fn test_create_then_start() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        client
            .create(proto::CreateRequest {
                name: "new-svc".to_string(),
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            })
            .await
            .unwrap();

        let resp = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.total_processes, 1);
        assert_eq!(resp.created_processes, 1);

        client
            .start(proto::StartRequest {
                name: "new-svc".to_string(),
            })
            .await
            .unwrap();

        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.processes[0].state, proto::ProcessState::Running as i32);
        assert!(resp.processes[0].pid > 0);

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(resp.processes[0].pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_create_duplicate_name() {
        let proc = ManagedProcess::new(
            "existing".to_string(),
            ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        );

        let (mut client, _shutdown) = start_test_server(vec![proc]).await;

        let err = client
            .create(proto::CreateRequest {
                name: "existing".to_string(),
                command: "/bin/sleep".to_string(),
                ..Default::default()
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::AlreadyExists);
    }

    #[tokio::test]
    async fn test_create_empty_command() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let err = client
            .create(proto::CreateRequest {
                name: "bad".to_string(),
                command: "".to_string(),
                ..Default::default()
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::InvalidArgument);
    }

    #[tokio::test]
    async fn test_create_defaults_applied() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        client
            .create(proto::CreateRequest {
                name: "defaults-svc".to_string(),
                command: "/bin/echo".to_string(),
                ..Default::default()
            })
            .await
            .unwrap();

        let resp = client
            .describe(proto::DescribeRequest {
                name: "defaults-svc".to_string(),
            })
            .await
            .unwrap()
            .into_inner();

        let detail = resp.detail.unwrap();
        assert_eq!(detail.command, "/bin/echo");
        assert!(detail.auto_start, "auto_start should default to true");
        assert_eq!(detail.stdout, "inherit");
        assert_eq!(detail.stderr, "inherit");
        assert_eq!(detail.restart_policy, "never");
        assert!(detail.args.is_empty());
        assert!(detail.env.is_empty());
    }

    #[tokio::test]
    async fn test_create_with_overrides() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let mut env = std::collections::HashMap::new();
        env.insert("MY_VAR".to_string(), "hello".to_string());

        client
            .create(proto::CreateRequest {
                name: "custom-svc".to_string(),
                command: "/usr/bin/custom".to_string(),
                args: vec!["--verbose".to_string()],
                env,
                working_dir: "/tmp".to_string(),
                stdout: "/var/log/out.log".to_string(),
                stderr: "/var/log/err.log".to_string(),
                restart_policy: "on-failure".to_string(),
                description: "Custom service".to_string(),
                auto_start: Some(false),
                ..Default::default()
            })
            .await
            .unwrap();

        let resp = client
            .describe(proto::DescribeRequest {
                name: "custom-svc".to_string(),
            })
            .await
            .unwrap()
            .into_inner();

        let detail = resp.detail.unwrap();
        assert_eq!(detail.command, "/usr/bin/custom");
        assert_eq!(detail.args, vec!["--verbose"]);
        assert_eq!(detail.env.get("MY_VAR").unwrap(), "hello");
        assert_eq!(detail.working_dir, "/tmp");
        assert_eq!(detail.stdout, "/var/log/out.log");
        assert_eq!(detail.stderr, "/var/log/err.log");
        assert_eq!(detail.restart_policy, "on-failure");
        assert_eq!(detail.description, "Custom service");
        assert!(!detail.auto_start);
    }
}
