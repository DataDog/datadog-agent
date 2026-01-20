pub const DEFAULT_GRPC_ADDR: &str = "127.0.0.1:50051";
pub const DEFAULT_GRPC_SOCKET: &str = "/var/run/datadog/process-manager.sock";

pub mod transport;

pub mod proto {
    tonic::include_proto!("process_manager");
}
