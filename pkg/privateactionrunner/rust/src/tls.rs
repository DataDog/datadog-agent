// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! mTLS for the control<->executor channel, reusing the agent IPC certificate.
//!
//! The agent stores a single self-signed IPC certificate file (a `CERTIFICATE`
//! PEM block followed by an `EC PRIVATE KEY` PEM block) that agent processes use
//! mutually: it is both the client identity par-control presents and the trust
//! root it uses to verify the executor (which presents the same cert). We use
//! native-tls (OpenSSL) here for the same cargo-deny reasons as the OPMS client.
//! Hostname verification is disabled because the channel runs over a local socket
//! with no meaningful peer hostname; the CA-chain + client-cert checks (the latter
//! enforced by the executor) provide the authentication.

use anyhow::{Context, Result, bail};
use std::path::Path;

/// Build a TLS connector that presents the IPC client identity and trusts the
/// IPC certificate as its root.
pub fn build_ipc_client_connector(ipc_cert_file: &Path) -> Result<tokio_native_tls::TlsConnector> {
    let pem = std::fs::read(ipc_cert_file)
        .with_context(|| format!("reading IPC cert file {}", ipc_cert_file.display()))?;
    let (cert_pem, key_pem) = split_cert_and_key(&pem)?;

    let identity = native_tls::Identity::from_pkcs8(&cert_pem, &key_pem)
        .context("building TLS identity from the IPC cert/key")?;
    let root = native_tls::Certificate::from_pem(&cert_pem)
        .context("parsing the IPC cert as a trust root")?;

    let connector = native_tls::TlsConnector::builder()
        .identity(identity)
        .add_root_certificate(root)
        // Local socket: no meaningful hostname to verify. Authentication is by the
        // shared IPC CA and (server-side) the required client certificate.
        .danger_accept_invalid_hostnames(true)
        .build()
        .context("building the native-tls connector")?;

    Ok(tokio_native_tls::TlsConnector::from(connector))
}

/// Split a combined IPC PEM into its CERTIFICATE and private-key blocks.
fn split_cert_and_key(pem: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    let text = std::str::from_utf8(pem).context("IPC cert file is not UTF-8")?;
    let cert = extract_block(text, "CERTIFICATE");
    // The IPC key block is "EC PRIVATE KEY" (SEC1); OpenSSL parses it fine.
    let key = extract_block(text, "EC PRIVATE KEY").or_else(|| extract_block(text, "PRIVATE KEY"));
    match (cert, key) {
        (Some(cert), Some(key)) => Ok((cert.into_bytes(), key.into_bytes())),
        (None, _) => bail!("IPC cert file has no CERTIFICATE block"),
        (_, None) => bail!("IPC cert file has no private key block"),
    }
}

/// Extract a single `-----BEGIN <label>----- ... -----END <label>-----` block.
fn extract_block(text: &str, label: &str) -> Option<String> {
    let begin = format!("-----BEGIN {label}-----");
    let end = format!("-----END {label}-----");
    let start = text.find(&begin)?;
    let stop = text[start..].find(&end)? + start + end.len();
    Some(text[start..stop].to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn splits_combined_pem() {
        let pem = b"prefix\n-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\nmiddle\n-----BEGIN EC PRIVATE KEY-----\nBBBB\n-----END EC PRIVATE KEY-----\ntrailing\n";
        let (cert, key) = split_cert_and_key(pem).unwrap();
        assert!(
            String::from_utf8(cert)
                .unwrap()
                .starts_with("-----BEGIN CERTIFICATE-----")
        );
        let key = String::from_utf8(key).unwrap();
        assert!(key.starts_with("-----BEGIN EC PRIVATE KEY-----"));
        assert!(key.ends_with("-----END EC PRIVATE KEY-----"));
    }

    #[test]
    fn errors_without_cert() {
        let pem = b"-----BEGIN EC PRIVATE KEY-----\nBBBB\n-----END EC PRIVATE KEY-----\n";
        assert!(split_cert_and_key(pem).is_err());
    }
}
