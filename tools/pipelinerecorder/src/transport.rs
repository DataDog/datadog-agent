use anyhow::Result;
use tokio::net::{UnixListener, UnixStream};
use std::path::Path;

/// A server-side Unix socket transport that accepts connections from the agent.
pub struct UnixSocketTransport {
    listener: UnixListener,
}

impl UnixSocketTransport {
    /// Bind a Unix socket at `path`. The file is removed first if it exists.
    pub fn bind(path: impl AsRef<Path>) -> Result<Self> {
        let path = path.as_ref();
        let _ = std::fs::remove_file(path);
        // Ensure parent directory exists.
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let listener = UnixListener::bind(path)?;
        Ok(Self { listener })
    }

    /// Accept the next connection, returning the stream.
    pub async fn accept_stream(&self) -> Result<UnixStream> {
        let (stream, _) = self.listener.accept().await?;
        Ok(stream)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::UnixStream;

    #[tokio::test]
    async fn test_connect_and_receive() {
        let dir = tempdir().unwrap();
        let sock = dir.path().join("test.sock");
        let transport = UnixSocketTransport::bind(&sock).unwrap();

        let sock_clone = sock.clone();
        tokio::spawn(async move {
            let mut client = UnixStream::connect(&sock_clone).await.unwrap();
            client.write_all(b"hello").await.unwrap();
        });

        let mut stream = transport.accept_stream().await.unwrap();
        let mut buf = vec![0u8; 5];
        stream.read_exact(&mut buf).await.unwrap();
        assert_eq!(&buf, b"hello");
    }

    #[tokio::test]
    async fn test_reconnect() {
        let dir = tempdir().unwrap();
        let sock = dir.path().join("reconnect.sock");
        let transport = UnixSocketTransport::bind(&sock).unwrap();

        // First client connects and sends data.
        let sock1 = sock.clone();
        tokio::spawn(async move {
            let mut c = UnixStream::connect(&sock1).await.unwrap();
            c.write_all(b"first").await.unwrap();
        });

        let mut s1 = transport.accept_stream().await.unwrap();
        let mut buf = vec![0u8; 5];
        s1.read_exact(&mut buf).await.unwrap();
        assert_eq!(&buf, b"first");
        drop(s1);

        // Second client connects.
        let sock2 = sock.clone();
        tokio::spawn(async move {
            let mut c = UnixStream::connect(&sock2).await.unwrap();
            c.write_all(b"second").await.unwrap();
        });

        let mut s2 = transport.accept_stream().await.unwrap();
        let mut buf2 = vec![0u8; 6];
        s2.read_exact(&mut buf2).await.unwrap();
        assert_eq!(&buf2, b"second");
    }
}
