use anyhow::{Context, Result};
use tokio::io::{AsyncRead, AsyncReadExt};

/// Read a single size-prefixed FlatBuffers frame from `reader`.
///
/// Wire format: [4-byte LE length] [payload bytes]
///
/// Returns `Ok(None)` on clean EOF (0 bytes read for the length prefix).
/// Returns `Err` on truncated frames or I/O errors.
pub async fn read_frame<R: AsyncRead + Unpin>(reader: &mut R) -> Result<Option<Vec<u8>>> {
    let mut len_buf = [0u8; 4];
    match reader.read_exact(&mut len_buf).await {
        Ok(_) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e).context("reading frame length"),
    }
    let len = u32::from_le_bytes(len_buf) as usize;
    if len == 0 {
        return Ok(Some(Vec::new()));
    }
    let mut buf = vec![0u8; len];
    reader
        .read_exact(&mut buf)
        .await
        .context("reading frame payload")?;
    Ok(Some(buf))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    fn make_frame(payload: &[u8]) -> Vec<u8> {
        let mut buf = Vec::with_capacity(4 + payload.len());
        buf.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        buf.extend_from_slice(payload);
        buf
    }

    #[tokio::test]
    async fn test_read_frame() {
        let frame = make_frame(b"hello");
        let mut cursor = Cursor::new(frame);
        let result = read_frame(&mut cursor).await.unwrap().unwrap();
        assert_eq!(result, b"hello");
    }

    #[tokio::test]
    async fn test_eof_returns_none() {
        let mut cursor = Cursor::new(Vec::<u8>::new());
        let result = read_frame(&mut cursor).await.unwrap();
        assert!(result.is_none());
    }

    #[tokio::test]
    async fn test_truncated_payload_returns_error() {
        // Length says 100 bytes but only 5 available.
        let mut buf = Vec::new();
        buf.extend_from_slice(&100u32.to_le_bytes());
        buf.extend_from_slice(b"short");
        let mut cursor = Cursor::new(buf);
        let result = read_frame(&mut cursor).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_two_frames_sequential() {
        let mut buf = make_frame(b"first");
        buf.extend_from_slice(&make_frame(b"second"));
        let mut cursor = Cursor::new(buf);

        let f1 = read_frame(&mut cursor).await.unwrap().unwrap();
        assert_eq!(f1, b"first");
        let f2 = read_frame(&mut cursor).await.unwrap().unwrap();
        assert_eq!(f2, b"second");
    }
}
