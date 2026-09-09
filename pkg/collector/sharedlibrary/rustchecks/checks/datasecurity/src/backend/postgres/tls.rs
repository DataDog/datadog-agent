//! Maps libpq `sslmode` onto an OpenSSL connector for the postgres engine.
//!
//! The `postgres` crate takes the TLS stack as an argument. We dynamically
//! link the Agent's already-shipped OpenSSL (`libssl`/`libcrypto`) instead of
//! compiling a second crypto library (aws-lc) into the check cdylib.

use anyhow::{Context, Result, bail};
use openssl::pkey::PKey;
use openssl::ssl::{SslConnector, SslConnectorBuilder, SslMethod, SslVerifyMode};
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

/// Builds the connector for the connection's `ssl`, or `None` when the mode
/// is `disable` and the caller should connect with `NoTls`.
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    if conn.ssl == SslMode::Disable {
        return Ok(None);
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;

    match conn.ssl {
        SslMode::Disable => unreachable!(),
        // Encrypt without authenticating the server, as libpq does for
        // `allow`, `prefer` and `require`.
        SslMode::Allow | SslMode::Prefer | SslMode::Require => {
            builder.set_verify(SslVerifyMode::NONE);
        }
        SslMode::VerifyCa | SslMode::VerifyFull => {
            builder.set_verify(SslVerifyMode::PEER);
            match conn.ssl_root_cert.as_deref() {
                Some(path) => builder
                    .set_ca_file(path)
                    .with_context(|| format!("reading ssl_root_cert {path}"))?,
                None => builder
                    .set_default_verify_paths()
                    .context("loading the system CA store")?,
            }
        }
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
    // `verify-ca` checks the chain but not the hostname; `verify-full` does both.
    if conn.ssl == SslMode::VerifyCa {
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

    fn conn(ssl: SslMode) -> Connection {
        Connection {
            ssl,
            ..Default::default()
        }
    }

    #[test]
    fn disable_asks_the_caller_for_no_tls() {
        assert!(connector(&conn(SslMode::Disable)).unwrap().is_none());
    }

    #[test]
    fn unverified_modes_need_no_trust_store() {
        for mode in [SslMode::Allow, SslMode::Prefer, SslMode::Require] {
            assert!(connector(&conn(mode)).unwrap().is_some());
        }
    }

    #[test]
    fn verify_modes_load_the_configured_root_cert() {
        let mut conn = conn(SslMode::VerifyFull);
        conn.ssl_root_cert = Some("/nonexistent/root.crt".to_string());
        assert!(connector(&conn).is_err());
    }

    #[test]
    fn rejects_incomplete_client_keys() {
        let mut cert_only = conn(SslMode::Require);
        cert_only.ssl_cert = Some("/nonexistent/client.crt".to_string());
        assert!(connector(&cert_only).is_err());
    }
}
