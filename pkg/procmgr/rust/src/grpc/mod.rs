// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub mod server;
pub mod service;

pub mod proto {
    tonic::include_proto!("datadog.procmgr");

    pub const FILE_DESCRIPTOR_SET: &[u8] =
        tonic::include_file_descriptor_set!("process_manager_descriptor");
}

#[cfg(test)]
mod tests {
    use super::proto;
    use super::proto::process_manager_client::ProcessManagerClient;
    use super::service::ProcessManagerService;
    use crate::ProcessManager;
    use crate::config::{ProcessConfig, RestartPolicy};
    use crate::process::ManagedProcess;
    use hyper_util::rt::TokioIo;
    use tokio::net::UnixListener;
    use tokio_stream::wrappers::UnixListenerStream;
    use tonic::transport::{Channel, Endpoint, Uri};
    use tower::service_fn;

    async fn start_test_server(
        processes: Vec<ManagedProcess>,
    ) -> (
        ProcessManagerClient<Channel>,
        tokio::sync::oneshot::Sender<()>,
    ) {
        let dir = tempfile::tempdir().unwrap();
        let sock_path = dir.path().join("test.sock");
        let uds = UnixListener::bind(&sock_path).unwrap();
        let uds_stream = UnixListenerStream::new(uds);

        let mgr = ProcessManager::new(processes);
        let svc = ProcessManagerService::new(mgr, "/test/config/path".to_string());

        let reflection = tonic_reflection::server::Builder::configure()
            .register_encoded_file_descriptor_set(proto::FILE_DESCRIPTOR_SET)
            .build_v1()
            .unwrap();

        let (shutdown_tx, shutdown_rx) = tokio::sync::oneshot::channel::<()>();

        tokio::spawn(async move {
            tonic::transport::Server::builder()
                .add_service(reflection)
                .add_service(proto::process_manager_server::ProcessManagerServer::new(
                    svc,
                ))
                .serve_with_incoming_shutdown(uds_stream, async {
                    let _ = shutdown_rx.await;
                })
                .await
                .unwrap();
            drop(dir);
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
        assert_eq!(detail.restart_policy, "OnFailure");
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
    }
}
