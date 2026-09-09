//! Maps libpq `sslmode` onto an OpenSSL connector for the postgres engine.
//!
//! The `postgres` crate takes the TLS stack as an argument. We dynamically
//! link the Agent's already-shipped OpenSSL (`libssl`/`libcrypto`) instead of
//! compiling a second crypto library (aws-lc) into the check cdylib.

use anyhow::{Context, Result, bail};
use openssl::ssl::{SslConnector, SslMethod, SslVerifyMode};
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

/// Builds the connector for the connection's `ssl`, or `None` when the mode
/// is `disable` and the caller should connect with `NoTls`.
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    match conn.ssl {
        SslMode::Disable => return Ok(None),
        SslMode::Allow | SslMode::Prefer | SslMode::Require => {}
        // TODO(DATASEC-156): support verify-ca / verify-full, and require's
        // upgrade to verify-ca when a CA file is configured.
        SslMode::VerifyCa => bail!("ssl mode `verify-ca` is unsupported"),
        SslMode::VerifyFull => bail!("ssl mode `verify-full` is unsupported"),
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;
    // Encrypt without authenticating the server. libpq does the same for
    // `allow` / `prefer` / `require` when no CA file is set.
    // TODO(DATASEC-156): set SslVerifyMode::PEER for verify-ca / verify-full
    // (and for require once a CA file is configured).
    builder.set_verify(SslVerifyMode::NONE);
    Ok(Some(MakeTlsConnector::new(builder.build())))
}
