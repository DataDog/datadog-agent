use anyhow::{Context, Result, bail};
use openssl::pkey::PKey;
use openssl::ssl::{SslConnector, SslConnectorBuilder, SslMethod, SslVerifyMode};
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

/// OpenSSL connector, or `None` for `disable` (`NoTls`).
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    if conn.ssl == SslMode::Disable {
        return Ok(None);
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;

    if matches!(conn.ssl, SslMode::VerifyCa | SslMode::VerifyFull) {
        builder.set_verify(SslVerifyMode::PEER);
        match conn.ssl_root_cert.as_deref() {
            Some(path) => builder
                .set_ca_file(path)
                .with_context(|| format!("reading ssl_root_cert {path}"))?,
            None => builder
                .set_default_verify_paths()
                .context("loading the system CA store")?,
        }
    } else {
        builder.set_verify(SslVerifyMode::NONE);
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
