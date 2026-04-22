//! Native messaging protocol helpers (4-byte length-prefixed JSON).
//!
//! Chrome native messaging uses a simple protocol:
//! - Each message is prefixed with a 4-byte little-endian unsigned integer length
//! - Followed by the UTF-8 JSON payload

use anyhow::{Context, Result};
use serde::Serialize;
use std::io::{self, Read, Write};

/// Chrome native messaging effectively caps payloads around 1 MiB; we align with that policy.
const MAX_MESSAGE_BYTES: usize = 1_048_576;
/// If the peer declares a length above [`MAX_MESSAGE_BYTES`] but below this cap, we discard the
/// body so the next read sees a fresh 4-byte length prefix. Above this cap we refuse to drain
/// (DoS) and the host must exit so the client can reconnect.
const MAX_DECLARED_LENGTH_TO_DRAIN: usize = 2 * 1024 * 1024;

/// Read a single message from stdin.
/// Returns Ok(None) on EOF (stdin closed).
pub fn read_message() -> Result<Option<serde_json::Value>> {
    let stdin = io::stdin();
    let mut handle = stdin.lock();
    read_message_from(&mut handle)
}

/// Like [`read_message`], but reads from any [`Read`] (used by unit tests).
pub(crate) fn read_message_from<R: Read>(handle: &mut R) -> Result<Option<serde_json::Value>> {
    // Read 4-byte length prefix (little-endian)
    let mut length_bytes = [0u8; 4];
    match handle.read_exact(&mut length_bytes) {
        Ok(()) => {}
        Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e).context("Failed to read message length"),
    }

    let message_length = u32::from_le_bytes(length_bytes) as usize;

    if message_length > MAX_MESSAGE_BYTES {
        if message_length > MAX_DECLARED_LENGTH_TO_DRAIN {
            anyhow::bail!(
                "Declared message length exceeds host limit: {} bytes (cannot resync)",
                message_length
            );
        }
        // Must consume the frame body before returning; otherwise the next read starts mid-payload
        // and native-messaging framing is permanently desynchronized.
        let mut remaining = message_length;
        let mut discard = [0u8; 8192];
        while remaining > 0 {
            let chunk = remaining.min(discard.len());
            handle.read_exact(&mut discard[..chunk]).with_context(|| {
                format!(
                    "Failed while discarding oversized message body ({} bytes total)",
                    message_length
                )
            })?;
            remaining -= chunk;
        }
        anyhow::bail!("Message too large: {} bytes", message_length);
    }

    // Read the JSON payload
    let mut message_bytes = vec![0u8; message_length];
    handle
        .read_exact(&mut message_bytes)
        .context("Failed to read message body")?;

    let value: serde_json::Value =
        serde_json::from_slice(&message_bytes).context("Failed to parse JSON message")?;

    Ok(Some(value))
}

/// Write a single message to stdout.
pub fn write_message<T: Serialize>(message: &T) -> Result<()> {
    let stdout = io::stdout();
    let mut handle = stdout.lock();

    let encoded = serde_json::to_vec(message).context("Failed to serialize response")?;

    // Write 4-byte length prefix (little-endian)
    let length_bytes = (encoded.len() as u32).to_le_bytes();
    handle
        .write_all(&length_bytes)
        .context("Failed to write message length")?;

    // Write the JSON payload
    handle
        .write_all(&encoded)
        .context("Failed to write message body")?;

    handle.flush().context("Failed to flush stdout")?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::{MAX_MESSAGE_BYTES, read_message_from};
    use std::io::Cursor;

    #[test]
    fn test_length_encoding() {
        let length: u32 = 42;
        let bytes = length.to_le_bytes();
        assert_eq!(bytes, [42, 0, 0, 0]);
        assert_eq!(u32::from_le_bytes(bytes), 42);
    }

    #[test]
    fn oversized_frame_is_drained_before_next_length_prefix() {
        let oversized = MAX_MESSAGE_BYTES + 7;
        let mut wire: Vec<u8> = Vec::new();
        wire.extend_from_slice(&(oversized as u32).to_le_bytes());
        wire.extend(vec![0u8; oversized]);
        let second = br#"{"type":"HEALTH_CHECK"}"#;
        wire.extend_from_slice(&(second.len() as u32).to_le_bytes());
        wire.extend_from_slice(second);

        let mut c = Cursor::new(wire);
        let err = read_message_from(&mut c).expect_err("expected oversize rejection");
        assert!(err.to_string().contains("Message too large"), "err={err:?}");

        let v = read_message_from(&mut c)
            .expect("second frame read")
            .expect("second frame present");
        assert_eq!(v["type"], "HEALTH_CHECK");
    }
}
