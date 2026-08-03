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
//!
//! The IPC key block is SEC1 (`EC PRIVATE KEY`, written by Go's
//! `x509.MarshalECPrivateKey`, see `pkg/api/security/cert/cert_generator.go`).
//! `native_tls::Identity::from_pkcs8`'s OpenSSL backend does a literal
//! `starts_with(b"-----BEGIN PRIVATE KEY-----")` check before ever parsing the
//! key, so it rejects SEC1 outright with `Error::NotPkcs8` even though OpenSSL's
//! own PEM parser would happily read it. We re-encode SEC1 -> PKCS8 ourselves via
//! the `openssl` crate (already linked in as a `native-tls` dependency) before
//! calling `from_pkcs8`.

use anyhow::{bail, Context, Result};
use openssl::pkey::PKey;
use std::path::Path;

/// Build a TLS connector that presents the IPC client identity and trusts the
/// IPC certificate as its root.
pub fn build_ipc_client_connector(ipc_cert_file: &Path) -> Result<tokio_native_tls::TlsConnector> {
    let pem = std::fs::read(ipc_cert_file)
        .with_context(|| format!("reading IPC cert file {}", ipc_cert_file.display()))?;
    let (cert_pem, key_pem) = split_cert_and_key(&pem)?;
    let key_pem = to_pkcs8_pem(&key_pem)?;

    let identity = native_tls::Identity::from_pkcs8(&cert_pem, &key_pem)
        .context("building TLS identity from the IPC cert/key")?;
    let root = native_tls::Certificate::from_pem(&cert_pem)
        .context("parsing the IPC cert as a trust root")?;

    let connector = native_tls::TlsConnector::builder()
        .identity(identity)
        .add_root_certificate(root)
        // tonic speaks HTTP/2. Unlike tonic's built-in TLS transport, a custom
        // native-tls connector does not request the h2 ALPN protocol implicitly;
        // without it, Go's gRPC server completes TLS and then closes the stream.
        .request_alpns(&["h2"])
        // Local socket: no meaningful hostname to verify. Authentication is by the
        // shared IPC CA and (server-side) the required client certificate.
        .danger_accept_invalid_hostnames(true)
        .build()
        .context("building the native-tls connector")?;

    Ok(tokio_native_tls::TlsConnector::from(connector))
}

/// Re-encode a private key PEM as PKCS8, whatever its original encoding.
///
/// `native_tls::Identity::from_pkcs8` requires the PKCS8 `-----BEGIN PRIVATE
/// KEY-----` header on its OpenSSL backend and rejects anything else
/// (including SEC1's `EC PRIVATE KEY`) without even attempting to parse it. See
/// the module doc comment. `PKey::private_key_from_pem` itself is tolerant of
/// both encodings, so this always succeeds for a well-formed EC key regardless
/// of which PEM header it originally had.
fn to_pkcs8_pem(key_pem: &[u8]) -> Result<Vec<u8>> {
    let pkey = PKey::private_key_from_pem(key_pem).context("parsing the IPC private key")?;
    pkey.private_key_to_pem_pkcs8()
        .context("re-encoding the IPC private key as PKCS8")
}

/// Split a combined IPC PEM into its CERTIFICATE and private-key blocks.
fn split_cert_and_key(pem: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    let text = std::str::from_utf8(pem).context("IPC cert file is not UTF-8")?;
    let cert = extract_block(text, "CERTIFICATE");
    // The IPC key block is "EC PRIVATE KEY" (SEC1). `to_pkcs8_pem` re-encodes it
    // before it reaches native_tls::Identity::from_pkcs8, which rejects SEC1.
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

/// Cert fixtures shared by this module's tests and `opms.rs`'s TLS round-trip
/// test (which needs a server identity to prove HTTPS works at all).
#[cfg(test)]
pub(crate) mod test_support {
    use super::*;

    /// Re-encode a key PEM as PKCS8, for callers that need to hand a key to
    /// `native_tls::Identity::from_pkcs8`.
    pub fn to_pkcs8(key_pem: &[u8]) -> Vec<u8> {
        to_pkcs8_pem(key_pem).expect("re-encoding the fixture key as PKCS8")
    }

    /// Generate a self-signed P-256 cert with a SEC1 key, matching the shape of
    /// the agent IPC cert written by `pkg/api/security/cert/cert_generator.go`.
    pub fn generate_self_signed_cert() -> (Vec<u8>, Vec<u8>) {
        use openssl::asn1::Asn1Time;
        use openssl::bn::{BigNum, MsbOption};
        use openssl::ec::{EcGroup, EcKey};
        use openssl::hash::MessageDigest;
        use openssl::nid::Nid;
        use openssl::x509::extension::{
            BasicConstraints, ExtendedKeyUsage, KeyUsage, SubjectAlternativeName,
        };
        use openssl::x509::{X509, X509NameBuilder};

        let group = EcGroup::from_curve_name(Nid::X9_62_PRIME256V1).unwrap();
        let ec_key = EcKey::generate(&group).unwrap();
        let pkey = PKey::from_ec_key(ec_key.clone()).unwrap();

        let mut name = X509NameBuilder::new().unwrap();
        name.append_entry_by_text("O", "Datadog, Inc.").unwrap();
        let name = name.build();

        let mut serial = BigNum::new().unwrap();
        serial.rand(128, MsbOption::MAYBE_ZERO, false).unwrap();

        let mut builder = X509::builder().unwrap();
        builder.set_version(2).unwrap();
        builder
            .set_serial_number(&serial.to_asn1_integer().unwrap())
            .unwrap();
        builder.set_subject_name(&name).unwrap();
        builder.set_issuer_name(&name).unwrap();
        builder.set_pubkey(&pkey).unwrap();
        builder
            .set_not_before(&Asn1Time::days_from_now(0).unwrap())
            .unwrap();
        builder
            .set_not_after(&Asn1Time::days_from_now(365 * 50).unwrap())
            .unwrap();
        builder
            .append_extension(BasicConstraints::new().ca().critical().build().unwrap())
            .unwrap();
        builder
            .append_extension(
                KeyUsage::new()
                    .key_cert_sign()
                    .digital_signature()
                    .crl_sign()
                    .critical()
                    .build()
                    .unwrap(),
            )
            .unwrap();
        builder
            .append_extension(
                ExtendedKeyUsage::new()
                    .server_auth()
                    .client_auth()
                    .build()
                    .unwrap(),
            )
            .unwrap();
        // Needed when the fixture is used as a *server* cert.
        builder
            .append_extension(
                SubjectAlternativeName::new()
                    .ip("127.0.0.1")
                    .dns("localhost")
                    .build(&builder.x509v3_context(None, None))
                    .unwrap(),
            )
            .unwrap();
        builder.sign(&pkey, MessageDigest::sha256()).unwrap();
        let cert = builder.build();

        let cert_pem = cert.to_pem().unwrap();
        // SEC1, not PKCS8 - matches x509.MarshalECPrivateKey on the Go side.
        let key_pem = ec_key.private_key_to_pem().unwrap();
        (cert_pem, key_pem)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn splits_combined_pem() {
        let pem = b"prefix\n-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\nmiddle\n-----BEGIN EC PRIVATE KEY-----\nBBBB\n-----END EC PRIVATE KEY-----\ntrailing\n";
        let (cert, key) = split_cert_and_key(pem).unwrap();
        assert!(String::from_utf8(cert)
            .unwrap()
            .starts_with("-----BEGIN CERTIFICATE-----"));
        let key = String::from_utf8(key).unwrap();
        assert!(key.starts_with("-----BEGIN EC PRIVATE KEY-----"));
        assert!(key.ends_with("-----END EC PRIVATE KEY-----"));
    }

    #[test]
    fn errors_without_cert() {
        let pem = b"-----BEGIN EC PRIVATE KEY-----\nBBBB\n-----END EC PRIVATE KEY-----\n";
        assert!(split_cert_and_key(pem).is_err());
    }

    /// De-risks the SEC1 (`EC PRIVATE KEY`) IPC key format: builds a real
    /// `native_tls`/OpenSSL connector from a generated IPC-style cert+key, the
    /// same shape `build_ipc_client_connector` reads from disk. Without the
    /// SEC1 -> PKCS8 re-encoding in `to_pkcs8_pem`, this fails with
    /// `native_tls::Error::NotPkcs8` because `Identity::from_pkcs8` string-checks
    /// for the PKCS8 PEM header before parsing.
    #[test]
    fn builds_connector_from_sec1_ipc_cert() {
        let (cert_pem, key_pem) = test_support::generate_self_signed_cert();
        assert!(
            String::from_utf8_lossy(&key_pem).starts_with("-----BEGIN EC PRIVATE KEY-----"),
            "fixture key is not SEC1-encoded, test would not exercise the SEC1 path"
        );

        let mut combined = cert_pem.clone();
        combined.extend_from_slice(&key_pem);

        let dir = tempfile::tempdir().unwrap();
        let cert_file = dir.path().join("ipc_cert.pem");
        std::fs::write(&cert_file, &combined).unwrap();

        build_ipc_client_connector(&cert_file)
            .expect("connector should build from a SEC1-keyed IPC cert");
    }
}
