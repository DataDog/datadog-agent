//! Native messaging protocol helpers (4-byte length-prefixed JSON).
//!
//! Chrome native messaging uses a simple protocol:
//! - Each message is prefixed with a 4-byte little-endian unsigned integer length
//! - Followed by the UTF-8 JSON payload

use anyhow::{Context, Result};
use serde::Serialize;
use std::io::{self, Read, Write};

/// Read a single message from stdin.
/// Returns Ok(None) on EOF (stdin closed).
pub fn read_message() -> Result<Option<serde_json::Value>> {
    let stdin = io::stdin();
    let mut handle = stdin.lock();

    // Read 4-byte length prefix (little-endian)
    let mut length_bytes = [0u8; 4];
    match handle.read_exact(&mut length_bytes) {
        Ok(()) => {}
        Err(e) if e.kind() == io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(e).context("Failed to read message length"),
    }

    let message_length = u32::from_le_bytes(length_bytes) as usize;

    // Sanity check: Chrome limits messages to 1MB
    if message_length > 1_048_576 {
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
    #[test]
    fn test_length_encoding() {
        let length: u32 = 42;
        let bytes = length.to_le_bytes();
        assert_eq!(bytes, [42, 0, 0, 0]);
        assert_eq!(u32::from_le_bytes(bytes), 42);
    }
}
