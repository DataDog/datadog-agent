// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Rustls setup for OPMS HTTPS and control<->executor mTLS.
//!
//! The provider selection and IPC certificate pinning follow Saluki's TLS
//! libraries: AWS-LC is used outside Windows and CNG on Windows. The Agent IPC
//! certificate is both the client identity and the exact server identity.

use anyhow::{Context, Result};
use rustls::crypto::CryptoProvider;
use std::{path::Path, sync::Arc};

/// Install Saluki's platform Rustls provider.
///
/// Installation is process-wide. Treat an already-installed provider as
/// success so independently constructed IPC and OPMS clients can share it.
pub fn initialize_crypto_provider() -> Result<()> {
    if CryptoProvider::get_default().is_some() {
        return Ok(());
    }

    match saluki_tls::initialize_default_crypto_provider() {
        Ok(()) => Ok(()),
        Err(_) if CryptoProvider::get_default().is_some() => Ok(()),
        Err(error) => Err(error).context("initializing Saluki's TLS crypto provider"),
    }
}

/// Build an mTLS connector from the Agent's combined `ipc_cert.pem`.
///
/// Saluki owns certificate parsing, exact certificate pinning, mutual
/// authentication, proof-of-possession verification, protocol selection, and
/// certificate-read retries. The caller rebuilds the connector for every new
/// connection so certificate creation and rotation are observed lazily.
pub async fn build_ipc_client_connector(
    ipc_cert_file: &Path,
) -> Result<tokio_rustls::TlsConnector> {
    initialize_crypto_provider()?;
    let mut config =
        datadog_agent_commons::ipc::tls::build_ipc_client_ipc_tls_config(ipc_cert_file)
            .await
            .with_context(|| {
                format!(
                    "building IPC mTLS configuration from {}",
                    ipc_cert_file.display()
                )
            })?;
    config.alpn_protocols = vec![b"h2".to_vec()];

    Ok(tokio_rustls::TlsConnector::from(Arc::new(config)))
}

#[cfg(test)]
pub(crate) mod test_support {
    use super::*;
    use rustls::{
        ServerConfig,
        pki_types::{CertificateDer, PrivateKeyDer},
    };
    use rustls_pki_types::pem::PemObject as _;

    pub const CERT_PEM: &[u8] = include_bytes!("../testdata/https-cert.txt");
    pub const KEY_PEM: &[u8] = include_bytes!("../testdata/https-key.txt");
    pub const IPC_IDENTITY_PEM: &[u8] = include_bytes!("../testdata/ipc-identity.txt");

    pub fn server_config() -> ServerConfig {
        initialize_crypto_provider().unwrap();
        let cert = CertificateDer::from_pem_slice(CERT_PEM)
            .unwrap()
            .into_owned();
        let key = PrivateKeyDer::from_pem_slice(KEY_PEM).unwrap().clone_key();
        let mut config = ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(vec![cert], key)
            .unwrap();
        config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];
        config
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn builds_connector_from_agent_sec1_identity() {
        assert!(
            std::str::from_utf8(test_support::IPC_IDENTITY_PEM)
                .unwrap()
                .contains("-----BEGIN EC PRIVATE KEY-----")
        );

        let dir = tempfile::tempdir().unwrap();
        let cert_file = dir.path().join("ipc_cert.pem");
        std::fs::write(&cert_file, test_support::IPC_IDENTITY_PEM).unwrap();

        build_ipc_client_connector(&cert_file)
            .await
            .expect("Rustls should accept the Agent's SEC1-keyed IPC identity");
    }
}
