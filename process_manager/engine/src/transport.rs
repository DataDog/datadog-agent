use std::convert::Infallible;
use std::env;
use std::error::Error;
use std::future::Future;
use std::net::SocketAddr;

use http::{Request as HttpRequest, Response as HttpResponse};
use hyper::Body;
use tonic::body::BoxBody;
use tonic::codegen::Service;
use tonic::server::NamedService;
use tonic::transport::{Channel, Server};

#[cfg(unix)]
use std::path::Path;
#[cfg(unix)]
use tokio::net::UnixListener;
#[cfg(unix)]
use tokio_stream::wrappers::UnixListenerStream;

#[cfg(unix)]
use tokio::net::UnixStream;
#[cfg(unix)]
use tonic::transport::{Endpoint, Uri};
#[cfg(unix)]
use tower::service_fn;

pub async fn serve_default<S, F>(service: S, shutdown: F) -> Result<(), Box<dyn Error>>
where
    S: Service<HttpRequest<Body>, Response = HttpResponse<BoxBody>, Error = Infallible>
        + NamedService
        + Clone
        + Send
        + 'static,
    S::Future: Send + 'static,
    F: Future<Output = ()> + Send + 'static,
{
    #[cfg(windows)]
    {
        let addr = tcp_addr_from_env()?;
        Server::builder()
            .add_service(service)
            .serve_with_shutdown(addr, shutdown)
            .await?;
        return Ok(());
    }

    #[cfg(unix)]
    {
        if env::var("DD_PM_USE_TCP").is_ok() {
            let addr = tcp_addr_from_env()?;
            Server::builder()
                .add_service(service)
                .serve_with_shutdown(addr, shutdown)
                .await?;
        } else {
            let socket_path = socket_path_from_env();
            serve_on_unix_socket(socket_path.as_str(), service, shutdown).await?;
        }
    }

    Ok(())
}

pub async fn create_channel_default() -> Result<Channel, Box<dyn Error>> {
    #[cfg(windows)]
    {
        let endpoint = format!("http://{}", tcp_addr_from_env()?);
        return Ok(Channel::from_shared(endpoint)?.connect().await?);
    }

    #[cfg(unix)]
    {
        if env::var("DD_PM_USE_TCP").is_ok() {
            let endpoint = format!("http://{}", tcp_addr_from_env()?);
            Ok(Channel::from_shared(endpoint)?.connect().await?)
        } else {
            let socket_path = socket_path_from_env();
            let channel = Endpoint::try_from("http://[::]:50051")?
                .connect_with_connector(service_fn(move |_: Uri| {
                    let socket_path = socket_path.clone();
                    async move { UnixStream::connect(socket_path).await }
                }))
                .await?;
            Ok(channel)
        }
    }
}

fn tcp_addr_from_env() -> Result<SocketAddr, Box<dyn Error>> {
    let addr = env::var("DD_PM_GRPC_ADDR")
        .unwrap_or_else(|_| crate::DEFAULT_GRPC_ADDR.to_string())
        .parse::<SocketAddr>()?;
    Ok(addr)
}

fn socket_path_from_env() -> String {
    env::var("DD_PM_GRPC_SOCKET")
        .or_else(|_| env::var("DD_PM_SOCKET"))
        .unwrap_or_else(|_| crate::DEFAULT_GRPC_SOCKET.to_string())
}

#[cfg(unix)]
async fn serve_on_unix_socket<S, F>(
    socket_path: &str,
    service: S,
    shutdown: F,
) -> Result<(), Box<dyn Error>>
where
    S: Service<HttpRequest<Body>, Response = HttpResponse<BoxBody>, Error = Infallible>
        + NamedService
        + Clone
        + Send
        + 'static,
    S::Future: Send + 'static,
    F: Future<Output = ()> + Send + 'static,
{
    let path = Path::new(socket_path);
    if path.exists() {
        let _ = std::fs::remove_file(path);
    }

    if let Some(parent) = path.parent() {
        if !parent.exists() {
            std::fs::create_dir_all(parent)?;
        }
    }

    let listener = UnixListener::bind(socket_path)?;

    {
        use std::os::unix::fs::PermissionsExt;
        let permissions = std::fs::Permissions::from_mode(0o660);
        std::fs::set_permissions(socket_path, permissions)?;
    }

    println!("Process Manager daemon listening on unix://{}", socket_path);

    Server::builder()
        .add_service(service)
        .serve_with_incoming_shutdown(UnixListenerStream::new(listener), shutdown)
        .await?;

    if path.exists() {
        let _ = std::fs::remove_file(path);
    }

    Ok(())
}
