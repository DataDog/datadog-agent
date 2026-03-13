// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub mod server;
pub mod service;

/// Placeholder URI for tonic Endpoint when connecting over UDS.
/// The actual address is irrelevant because `connect_with_connector` bypasses it.
pub const UDS_DUMMY_ENDPOINT: &str = "http://[::]:50051";

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
    use crate::command::Command;
    use crate::config::{ProcessConfig, ProcessDefinition, RestartPolicy, StaticConfigLoader};
    use crate::manager::ProcessManager;
    use hyper_util::rt::TokioIo;
    use std::sync::Arc;
    use tokio::net::UnixListener;
    use tokio::sync::mpsc;
    use tokio_stream::wrappers::UnixListenerStream;
    use tonic::transport::{Channel, Endpoint, Uri};
    use tower::service_fn;

    async fn start_test_server(
        defs: Vec<ProcessDefinition>,
    ) -> (
        ProcessManagerClient<Channel>,
        tokio::sync::oneshot::Sender<()>,
    ) {
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Command>(64);
        let dir = tempfile::tempdir().unwrap();
        let sock_path = dir.path().join("test.sock");
        let uds = UnixListener::bind(&sock_path).unwrap();
        let uds_stream = UnixListenerStream::new(uds);

        let mgr = ProcessManager::new(Arc::new(StaticConfigLoader::new(defs)));
        let svc = ProcessManagerService::new(mgr.clone(), cmd_tx);

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
                                    mgr_loop.handle_create(name, *config, &exit_tx_loop).await,
                                );
                            }
                            Command::Start { name_or_uuid, reply } => {
                                let _ = reply.send(
                                    mgr_loop.handle_start(&name_or_uuid, &exit_tx_loop).await,
                                );
                            }
                            Command::Stop { name_or_uuid, reply } => {
                                let _ = reply.send(mgr_loop.handle_stop(&name_or_uuid).await);
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

        let channel = Endpoint::from_static(super::UDS_DUMMY_ENDPOINT)
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
        let defs = vec![
            ProcessDefinition {
                name: "alpha".to_string(),
                config: ProcessConfig {
                    command: "/usr/bin/alpha".to_string(),
                    args: vec!["--flag".to_string()],
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "beta".to_string(),
                config: ProcessConfig {
                    command: "/usr/bin/beta".to_string(),
                    ..Default::default()
                },
            },
        ];

        let (mut client, _shutdown) = start_test_server(defs).await;
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

        let defs = vec![ProcessDefinition {
            name: "my-service".to_string(),
            config: ProcessConfig {
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
        }];

        let (mut client, _shutdown) = start_test_server(defs).await;
        let resp = client
            .describe(proto::DescribeRequest {
                name_or_uuid: "my-service".to_string(),
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
                name_or_uuid: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();

        assert_eq!(status.code(), tonic::Code::NotFound);
        assert_eq!(status.message(), "process 'nonexistent' not found");
    }

    #[tokio::test]
    async fn test_get_status() {
        let defs = vec![
            ProcessDefinition {
                name: "running-proc".to_string(),
                config: ProcessConfig {
                    command: "/bin/true".to_string(),
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "another-proc".to_string(),
                config: ProcessConfig {
                    command: "/bin/false".to_string(),
                    ..Default::default()
                },
            },
        ];

        let (mut client, _shutdown) = start_test_server(defs).await;
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
    }

    #[tokio::test]
    async fn test_get_config() {
        let defs = vec![
            ProcessDefinition {
                name: "svc-a".to_string(),
                config: ProcessConfig {
                    command: "/bin/true".to_string(),
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "svc-b".to_string(),
                config: ProcessConfig {
                    command: "/bin/false".to_string(),
                    ..Default::default()
                },
            },
        ];

        let (mut client, _shutdown) = start_test_server(defs).await;
        let resp = client
            .get_config(proto::GetConfigRequest {})
            .await
            .unwrap()
            .into_inner();

        assert_eq!(resp.source, "static");
        assert_eq!(resp.location, "in-memory (test)");
        assert_eq!(resp.loaded_processes, 2);
        assert_eq!(resp.runtime_processes, 0);
    }

    #[tokio::test]
    async fn test_get_config_empty() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;
        let resp = client
            .get_config(proto::GetConfigRequest {})
            .await
            .unwrap()
            .into_inner();

        assert_eq!(resp.source, "static");
        assert_eq!(resp.location, "in-memory (test)");
        assert_eq!(resp.loaded_processes, 0);
        assert_eq!(resp.runtime_processes, 0);
    }

    #[tokio::test]
    async fn test_get_config_reflects_runtime_creates() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }])
        .await;

        let resp = client
            .get_config(proto::GetConfigRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.loaded_processes, 1);
        assert_eq!(resp.runtime_processes, 0);

        client
            .create(proto::CreateRequest {
                name: "dynamic-svc".to_string(),
                command: "/bin/echo".to_string(),
                ..Default::default()
            })
            .await
            .unwrap();

        let resp = client
            .get_config(proto::GetConfigRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.loaded_processes, 1);
        assert_eq!(resp.runtime_processes, 1);
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
        let defs = vec![
            ProcessDefinition {
                name: "alpha".to_string(),
                config: ProcessConfig {
                    command: "/usr/bin/alpha".to_string(),
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "beta".to_string(),
                config: ProcessConfig {
                    command: "/usr/bin/beta".to_string(),
                    description: Some("The beta service".to_string()),
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "gamma".to_string(),
                config: ProcessConfig {
                    command: "/usr/bin/gamma".to_string(),
                    ..Default::default()
                },
            },
        ];

        let (mut client, _shutdown) = start_test_server(defs).await;

        let resp = client
            .describe(proto::DescribeRequest {
                name_or_uuid: "beta".to_string(),
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
                name_or_uuid: "gamma".to_string(),
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
        let defs = vec![
            ProcessDefinition {
                name: "running-svc".to_string(),
                config: ProcessConfig {
                    command: "/bin/sleep".to_string(),
                    args: vec!["60".to_string()],
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "failed-svc".to_string(),
                config: ProcessConfig {
                    command: "/bin/sh".to_string(),
                    args: vec!["-c".to_string(), "exit 1".to_string()],
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "stopped-svc".to_string(),
                config: ProcessConfig {
                    command: "/bin/sleep".to_string(),
                    args: vec!["60".to_string()],
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "exited-svc".to_string(),
                config: ProcessConfig {
                    command: "/bin/sh".to_string(),
                    args: vec!["-c".to_string(), "exit 0".to_string()],
                    ..Default::default()
                },
            },
            ProcessDefinition {
                name: "created-svc".to_string(),
                config: ProcessConfig {
                    command: "/bin/true".to_string(),
                    ..Default::default()
                },
            },
        ];
        let (mut client, _shutdown) = start_test_server(defs).await;

        // Running: start a long-lived process
        client
            .start(proto::StartRequest {
                name_or_uuid: "running-svc".to_string(),
            })
            .await
            .unwrap();

        // Failed: start a process that exits non-zero (watcher delivers the exit event)
        client
            .start(proto::StartRequest {
                name_or_uuid: "failed-svc".to_string(),
            })
            .await
            .unwrap();

        // Stopped: start then stop
        client
            .start(proto::StartRequest {
                name_or_uuid: "stopped-svc".to_string(),
            })
            .await
            .unwrap();
        client
            .stop(proto::StopRequest {
                name_or_uuid: "stopped-svc".to_string(),
            })
            .await
            .unwrap();

        // Exited: start a process that exits cleanly
        client
            .start(proto::StartRequest {
                name_or_uuid: "exited-svc".to_string(),
            })
            .await
            .unwrap();

        // Wait for fast processes to exit and events to flow
        tokio::time::sleep(std::time::Duration::from_millis(500)).await;

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

        // Clean up running-svc
        let list = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        if let Some(p) = list.processes.iter().find(|p| p.name == "running-svc") {
            nix::sys::signal::kill(
                nix::unistd::Pid::from_raw(p.pid as i32),
                nix::sys::signal::Signal::SIGKILL,
            )
            .ok();
        }
    }

    #[tokio::test]
    async fn test_list_shows_running_pid() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "live-proc".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }])
        .await;

        client
            .start(proto::StartRequest {
                name_or_uuid: "live-proc".to_string(),
            })
            .await
            .unwrap();

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

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(resp.processes[0].pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    // -- Write RPC tests --

    #[tokio::test]
    async fn test_start_rpc_success() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "sleeper".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }])
        .await;

        let start_resp = client
            .start(proto::StartRequest {
                name_or_uuid: "sleeper".to_string(),
            })
            .await
            .unwrap()
            .into_inner();

        assert_eq!(
            start_resp.state,
            proto::ProcessState::Running as i32,
            "start response should report actual Running state"
        );
        assert!(start_resp.pid > 0, "start response should include pid");
        assert!(
            !start_resp.uuid.is_empty(),
            "start response should include uuid"
        );

        // Cross-check via list
        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.processes[0].state, proto::ProcessState::Running as i32);
        assert_eq!(resp.processes[0].pid, start_resp.pid);

        // Clean up
        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(start_resp.pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_start_rpc_not_found() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let err = client
            .start(proto::StartRequest {
                name_or_uuid: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_start_rpc_already_running() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "running".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }])
        .await;

        client
            .start(proto::StartRequest {
                name_or_uuid: "running".to_string(),
            })
            .await
            .unwrap();

        let err = client
            .start(proto::StartRequest {
                name_or_uuid: "running".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::FailedPrecondition);

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw({
                let resp = client
                    .list(proto::ListRequest {})
                    .await
                    .unwrap()
                    .into_inner();
                resp.processes[0].pid as i32
            }),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_stop_rpc_success() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "to-stop".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }])
        .await;

        // Start via RPC so the watcher is wired
        client
            .start(proto::StartRequest {
                name_or_uuid: "to-stop".to_string(),
            })
            .await
            .unwrap();

        let stop_resp = client
            .stop(proto::StopRequest {
                name_or_uuid: "to-stop".to_string(),
            })
            .await
            .unwrap()
            .into_inner();

        assert_eq!(
            stop_resp.state,
            proto::ProcessState::Stopped as i32,
            "stop response should report actual Stopped state"
        );
        assert!(
            !stop_resp.uuid.is_empty(),
            "stop response should include uuid"
        );

        // Cross-check via describe
        tokio::time::sleep(std::time::Duration::from_millis(200)).await;

        let resp = client
            .describe(proto::DescribeRequest {
                name_or_uuid: "to-stop".to_string(),
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
                name_or_uuid: "nonexistent".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_stop_rpc_not_running() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "idle".to_string(),
            config: ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }])
        .await;

        let err = client
            .stop(proto::StopRequest {
                name_or_uuid: "idle".to_string(),
            })
            .await
            .unwrap_err();
        assert_eq!(err.code(), tonic::Code::FailedPrecondition);
    }

    #[tokio::test]
    async fn test_start_then_stop_round_trip() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "lifecycle".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }])
        .await;

        // Start
        client
            .start(proto::StartRequest {
                name_or_uuid: "lifecycle".to_string(),
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
                name_or_uuid: "lifecycle".to_string(),
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
                auto_start: Some(false),
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
                name_or_uuid: "new-svc".to_string(),
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
    async fn test_create_auto_start() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        client
            .create(proto::CreateRequest {
                name: "auto-svc".to_string(),
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: Some(true),
                ..Default::default()
            })
            .await
            .unwrap();

        tokio::time::sleep(std::time::Duration::from_millis(200)).await;

        let resp = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.processes.len(), 1);
        assert_eq!(resp.processes[0].state, proto::ProcessState::Running as i32);
        assert!(
            resp.processes[0].pid > 0,
            "auto-started process should have a PID"
        );

        nix::sys::signal::kill(
            nix::unistd::Pid::from_raw(resp.processes[0].pid as i32),
            nix::sys::signal::Signal::SIGKILL,
        )
        .ok();
    }

    #[tokio::test]
    async fn test_create_auto_start_false_stays_created() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        client
            .create(proto::CreateRequest {
                name: "manual-svc".to_string(),
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                auto_start: Some(false),
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
        assert_eq!(
            resp.created_processes, 1,
            "auto_start=false should leave process in created state"
        );
        assert_eq!(resp.running_processes, 0);
    }

    #[tokio::test]
    async fn test_create_auto_start_bad_command() {
        let (mut client, _shutdown) = start_test_server(vec![]).await;

        let resp = client
            .create(proto::CreateRequest {
                name: "bad-cmd".to_string(),
                command: "/nonexistent/binary".to_string(),
                auto_start: Some(true),
                ..Default::default()
            })
            .await
            .unwrap()
            .into_inner();
        assert!(!resp.uuid.is_empty(), "process should still be created");

        let status = client
            .get_status(proto::GetStatusRequest {})
            .await
            .unwrap()
            .into_inner();
        assert_eq!(status.total_processes, 1);
        assert_eq!(
            status.running_processes, 0,
            "process with bad command should not be running"
        );
    }

    #[tokio::test]
    async fn test_create_duplicate_name() {
        let (mut client, _shutdown) = start_test_server(vec![ProcessDefinition {
            name: "existing".to_string(),
            config: ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }])
        .await;

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
                name_or_uuid: "defaults-svc".to_string(),
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
                name_or_uuid: "custom-svc".to_string(),
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

    // -- UUID prefix resolution tests --

    #[tokio::test]
    async fn test_describe_by_uuid_prefix() {
        let defs = vec![ProcessDefinition {
            name: "svc-a".to_string(),
            config: ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }];
        let (mut client, _shutdown) = start_test_server(defs).await;

        let list = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        let full_uuid = &list.processes[0].uuid;
        let prefix = &full_uuid[..8];

        let resp = client
            .describe(proto::DescribeRequest {
                name_or_uuid: prefix.to_string(),
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.detail.unwrap().name, "svc-a");
    }

    #[tokio::test]
    async fn test_start_stop_by_uuid_prefix() {
        let defs = vec![ProcessDefinition {
            name: "svc-b".to_string(),
            config: ProcessConfig {
                command: "/bin/sleep".to_string(),
                args: vec!["60".to_string()],
                ..Default::default()
            },
        }];
        let (mut client, _shutdown) = start_test_server(defs).await;

        let list = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        let prefix = list.processes[0].uuid[..8].to_string();

        let start_resp = client
            .start(proto::StartRequest {
                name_or_uuid: prefix.clone(),
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(start_resp.state, proto::ProcessState::Running as i32);
        assert!(start_resp.pid > 0);

        let stop_resp = client
            .stop(proto::StopRequest {
                name_or_uuid: prefix,
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(stop_resp.state, proto::ProcessState::Stopped as i32);
    }

    #[tokio::test]
    async fn test_describe_by_full_uuid() {
        let defs = vec![ProcessDefinition {
            name: "svc-c".to_string(),
            config: ProcessConfig {
                command: "/bin/true".to_string(),
                ..Default::default()
            },
        }];
        let (mut client, _shutdown) = start_test_server(defs).await;

        let list = client
            .list(proto::ListRequest {})
            .await
            .unwrap()
            .into_inner();
        let full_uuid = list.processes[0].uuid.clone();

        let resp = client
            .describe(proto::DescribeRequest {
                name_or_uuid: full_uuid,
            })
            .await
            .unwrap()
            .into_inner();
        assert_eq!(resp.detail.unwrap().name, "svc-c");
    }
}
