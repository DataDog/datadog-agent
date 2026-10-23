use anyhow::{Context, Result, bail};
use openssl::pkey::PKey;
use openssl::ssl::{SslConnector, SslConnectorBuilder, SslMethod, SslVerifyMode};
use openssl::x509::store::X509StoreBuilder;
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

#[derive(Debug, PartialEq, Eq)]
enum RootCert<'a> {
    System,
    File(&'a str),
}

/// CAs to verify the server certificate with, or `None` when it isn't verified.
///
/// Like libpq, `require` verifies the certificate chain (not the host name) when a root
/// certificate is provided. This is what the postgres integration documents for `ssl`, see
/// https://github.com/DataDog/integrations-core/blob/e5b2b9c9e1bcb2caa1a3de3a44a11bfa3678d6aa/postgres/assets/configuration/spec.yaml#L197-L198
fn verification_root_cert(conn: &Connection) -> Option<RootCert<'_>> {
    let verify = matches!(conn.ssl, SslMode::VerifyCa | SslMode::VerifyFull)
        || (conn.ssl == SslMode::Require && conn.ssl_root_cert.is_some());
    if !verify {
        return None;
    }
    Some(match conn.ssl_root_cert.as_deref() {
        Some(path) => RootCert::File(path),
        None => RootCert::System,
    })
}

/// OpenSSL connector, or `None` for `disable` (`NoTls`).
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    if conn.ssl == SslMode::Disable {
        return Ok(None);
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;

    match verification_root_cert(conn) {
        // set_ca_file adds to the builder's system store; replace it so we trust only this CA.
        Some(RootCert::File(path)) => {
            builder.set_verify(SslVerifyMode::PEER);
            builder.set_cert_store(
                X509StoreBuilder::new()
                    .context("creating CA store")?
                    .build(),
            );
            builder
                .set_ca_file(path)
                .with_context(|| format!("reading ssl_root_cert {path}"))?;
        }
        Some(RootCert::System) => {
            builder.set_verify(SslVerifyMode::PEER);
            builder
                .set_default_verify_paths()
                .context("loading the system CA store")?;
        }
        None => builder.set_verify(SslVerifyMode::NONE),
    }

    match (&conn.ssl_cert, &conn.ssl_key) {
        (Some(cert), Some(key)) => {
            builder
                .set_certificate_chain_file(cert)
                .with_context(|| format!("reading ssl_cert {cert}"))?;
            load_private_key(&mut builder, key, conn.ssl_password.as_deref())?;
        }
        (None, None) => {}
        _ => bail!("ssl_cert and ssl_key must be set together"),
    }

    let mut tls = MakeTlsConnector::new(builder.build());
    // postgres-openssl checks the host name by default; only `verify-full` wants that.
    if conn.ssl != SslMode::VerifyFull {
        tls.set_callback(|ssl, _domain| {
            ssl.set_verify_hostname(false);
            Ok(())
        });
    }
    Ok(Some(tls))
}

fn load_private_key(
    builder: &mut SslConnectorBuilder,
    path: &str,
    password: Option<&str>,
) -> Result<()> {
    let pem = std::fs::read(path).with_context(|| format!("reading ssl_key {path}"))?;
    let pkey = match password {
        Some(pass) => PKey::private_key_from_pem_passphrase(&pem, pass.as_bytes()),
        None => PKey::private_key_from_pem(&pem),
    }
    .with_context(|| format!("parsing ssl_key {path}"))?;
    builder
        .set_private_key(&pkey)
        .with_context(|| format!("loading ssl_key {path}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn conn(ssl: SslMode, ssl_root_cert: Option<&str>) -> Connection {
        Connection {
            ssl,
            ssl_root_cert: ssl_root_cert.map(str::to_string),
            ..Default::default()
        }
    }

    #[test]
    fn disable_uses_no_tls() {
        assert!(connector(&conn(SslMode::Disable, None)).unwrap().is_none());
    }

    #[test]
    fn require_without_root_cert_does_not_verify() {
        assert!(connector(&conn(SslMode::Require, None)).unwrap().is_some());
    }

    #[test]
    fn require_with_root_cert_verifies_like_libpq() {
        // The root certificate is loaded, so a missing file is an error.
        let err = connector(&conn(SslMode::Require, Some("/nonexistent/root.crt")))
            .err()
            .unwrap();
        assert!(format!("{err:#}").contains("ssl_root_cert"));
    }

    #[test]
    fn non_verifying_modes_do_not_verify_even_with_a_root_cert() {
        for mode in [SslMode::Disable, SslMode::Allow, SslMode::Prefer] {
            assert_eq!(verification_root_cert(&conn(mode, None)), None);
            assert_eq!(verification_root_cert(&conn(mode, Some("/ca.crt"))), None);
        }
    }

    #[test]
    fn require_verifies_only_with_a_root_cert() {
        assert_eq!(verification_root_cert(&conn(SslMode::Require, None)), None);
        assert_eq!(
            verification_root_cert(&conn(SslMode::Require, Some("/ca.crt"))),
            Some(RootCert::File("/ca.crt"))
        );
    }

    #[test]
    fn verify_modes_use_the_root_cert_file() {
        for mode in [SslMode::VerifyCa, SslMode::VerifyFull] {
            assert_eq!(
                verification_root_cert(&conn(mode, Some("/ca.crt"))),
                Some(RootCert::File("/ca.crt"))
            );
        }
    }

    #[test]
    fn verify_modes_without_root_cert_use_the_system_store() {
        for mode in [SslMode::VerifyCa, SslMode::VerifyFull] {
            assert_eq!(
                verification_root_cert(&conn(mode, None)),
                Some(RootCert::System)
            );
            assert!(connector(&conn(mode, None)).unwrap().is_some());
        }
    }
}
