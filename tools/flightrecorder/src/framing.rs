use anyhow::{Context, Result};
use tokio::io::{AsyncRead, AsyncReadExt};

/// Read a single size-prefixed FlatBuffers frame from `reader` into `buf`.
///
/// Wire format: [4-byte LE length] [payload bytes]
///
/// The caller owns `buf` and it is reused across calls — `buf.resize()` reuses
/// capacity for same-size or smaller frames, avoiding per-frame allocation.
///
/// Returns `Ok(Some(len))` on success (payload length), `Ok(None)` on clean
/// EOF, or `Err` on truncated frames / I/O errors.
pub async fn read_frame<R: AsyncRead + Unpin>(
    reader: &mut R,
    buf: &mut Vec<u8>,
) -> Result<Option<usize>> {
    let mut len_buf = [0u8; 4];
    match reader.read_exact(&mut len_buf).await {
        Ok(_) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e).context("reading frame length"),
    }
    let len = u32::from_le_bytes(len_buf) as usize;
    if len == 0 {
        buf.clear();
        return Ok(Some(0));
    }
    buf.resize(len, 0);
    reader
        .read_exact(buf)
        .await
        .context("reading frame payload")?;
    Ok(Some(len))
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
        let mut buf = Vec::new();
        let len = read_frame(&mut cursor, &mut buf).await.unwrap().unwrap();
        assert_eq!(len, 5);
        assert_eq!(buf, b"hello");
    }

    #[tokio::test]
    async fn test_eof_returns_none() {
        let mut cursor = Cursor::new(Vec::<u8>::new());
        let mut buf = Vec::new();
        let result = read_frame(&mut cursor, &mut buf).await.unwrap();
        assert!(result.is_none());
    }

    #[tokio::test]
    async fn test_truncated_payload_returns_error() {
        // Length says 100 bytes but only 5 available.
        let mut raw = Vec::new();
        raw.extend_from_slice(&100u32.to_le_bytes());
        raw.extend_from_slice(b"short");
        let mut cursor = Cursor::new(raw);
        let mut buf = Vec::new();
        let result = read_frame(&mut cursor, &mut buf).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_two_frames_sequential() {
        let mut raw = make_frame(b"first");
        raw.extend_from_slice(&make_frame(b"second"));
        let mut cursor = Cursor::new(raw);
        let mut buf = Vec::new();

        let len1 = read_frame(&mut cursor, &mut buf).await.unwrap().unwrap();
        assert_eq!(len1, 5);
        assert_eq!(buf, b"first");

        let len2 = read_frame(&mut cursor, &mut buf).await.unwrap().unwrap();
        assert_eq!(len2, 6);
        assert_eq!(buf, b"second");
    }

    #[tokio::test]
    async fn test_buffer_reuse() {
        // Read a large frame, then a small one — capacity should be reused.
        let mut raw = make_frame(&[0xAA; 1000]);
        raw.extend_from_slice(&make_frame(&[0xBB; 10]));
        let mut cursor = Cursor::new(raw);
        let mut buf = Vec::new();

        read_frame(&mut cursor, &mut buf).await.unwrap().unwrap();
        assert_eq!(buf.len(), 1000);
        let cap_after_large = buf.capacity();

        read_frame(&mut cursor, &mut buf).await.unwrap().unwrap();
        assert_eq!(buf.len(), 10);
        // Capacity should not have shrunk — the old allocation is reused.
        assert_eq!(buf.capacity(), cap_after_large);
    }
}
