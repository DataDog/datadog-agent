//! Maps libpq `sslmode` onto an OpenSSL connector for the postgres engine.
//!
//! The `postgres` crate takes the TLS stack as an argument. We dynamically
//! link the Agent's already-shipped OpenSSL (`libssl`/`libcrypto`) instead of
//! compiling a second crypto library (aws-lc) into the check cdylib.

use anyhow::{Context, Result, bail};
use openssl::ssl::{SslConnector, SslMethod, SslVerifyMode};
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

/// OpenSSL connector, or `None` for `disable` (`NoTls`).
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    match conn.ssl {
        SslMode::Disable => return Ok(None),
        // TODO(DATASEC-156): verify-ca / verify-full (SslVerifyMode::PEER + CA).
        SslMode::VerifyCa => bail!("ssl mode `verify-ca` is unsupported"),
        SslMode::VerifyFull => bail!("ssl mode `verify-full` is unsupported"),
        _ => {}
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;
    builder.set_verify(SslVerifyMode::NONE);
    Ok(Some(MakeTlsConnector::new(builder.build())))
}
