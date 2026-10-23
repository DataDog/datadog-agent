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

    fn testdata(name: &str) -> String {
        let dir = std::env::var("CARGO_MANIFEST_DIR").unwrap();
        format!("{dir}/testdata/{name}")
    }

    fn conn(ssl: SslMode, ssl_root_cert: Option<&str>) -> Connection {
        Connection {
            ssl,
            ssl_root_cert: ssl_root_cert.map(str::to_string),
            ..Default::default()
        }
    }

    /// What `~/.postgresql/root.crt` is.
    #[derive(Debug, Clone, Copy)]
    enum Home {
        /// No home directory.
        Unknown,
        WithRoot,
        WithoutRoot,
    }

    /// The `ssl_root_cert` setting.
    #[derive(Debug, Clone, Copy)]
    enum Cert {
        Unset,
        Empty,
        System,
        /// `./system` is a file, not the system store.
        RelativeSystem,
        Valid,
        Missing,
    }

    impl Cert {
        fn value(self) -> Option<String> {
            match self {
                Cert::Unset => None,
                Cert::Empty => Some(String::new()),
                Cert::System => Some("system".to_string()),
                Cert::RelativeSystem => Some("./system".to_string()),
                Cert::Valid => Some(testdata("root.crt")),
                Cert::Missing => Some("/nonexistent/root.crt".to_string()),
            }
        }
    }

    /// The expected root certificate.
    #[derive(Debug)]
    enum Want {
        Unverified,
        SystemStore,
        /// The file the setting points to.
        File(Cert),
    }

    /// The selected root certificate.
    #[derive(Debug)]
    enum Got {
        Unverified,
        SystemStore,
        File(String),
    }

    impl Want {
        fn matches(&self, got: &Got) -> bool {
            match (self, got) {
                (Want::Unverified, Got::Unverified) | (Want::SystemStore, Got::SystemStore) => true,
                (Want::File(cert), Got::File(path)) => cert.value().as_deref() == Some(path),
                _ => false,
            }
        }
    }

    fn select(ssl: SslMode, cert: Cert, _home: Home) -> Got {
        match verification_root_cert(&conn(ssl, cert.value().as_deref())) {
            None => Got::Unverified,
            Some(RootCert::System) => Got::SystemStore,
            Some(RootCert::File(path)) => Got::File(path.to_string()),
        }
    }

    #[test]
    fn root_cert_selection_matrix() {
        use Cert::*;
        use Home::*;
        use SslMode::*;
        use Want::*;

        #[rustfmt::skip]
        let cases = [
            // ssl         ssl_root_cert   ~/.postgresql/root.crt   selected
            // Non-verifying modes ignore the root certificate.
            (Disable,      Unset,          WithRoot,                Unverified),
            (Disable,      Valid,          WithRoot,                Unverified),
            (Disable,      Missing,        WithRoot,                Unverified),
            (Disable,      System,         Unknown,                 Unverified),
            (Allow,        Unset,          WithRoot,                Unverified),
            (Allow,        Valid,          WithRoot,                Unverified),
            (Allow,        Missing,        WithRoot,                Unverified),
            (Allow,        System,         Unknown,                 Unverified),
            (Prefer,       Unset,          WithRoot,                Unverified),
            (Prefer,       Valid,          WithRoot,                Unverified),
            (Prefer,       Missing,        WithRoot,                Unverified),
            (Prefer,       System,         Unknown,                 Unverified),

            (Require,      Unset,          Unknown,                 Unverified),
            (Require,      Unset,          WithoutRoot,             Unverified),
            (Require,      Unset,          WithRoot,                Unverified),
            (Require,      Empty,          WithRoot,                File(Empty)),
            (Require,      Valid,          WithoutRoot,             File(Valid)),
            (Require,      Missing,        WithRoot,                File(Missing)),
            (Require,      RelativeSystem, WithRoot,                File(RelativeSystem)),
            (Require,      System,         WithRoot,                File(System)),

            (VerifyCa,     Unset,          Unknown,                 SystemStore),
            (VerifyCa,     Unset,          WithoutRoot,             SystemStore),
            (VerifyCa,     Unset,          WithRoot,                SystemStore),
            (VerifyCa,     Empty,          WithRoot,                File(Empty)),
            (VerifyCa,     Valid,          WithoutRoot,             File(Valid)),
            (VerifyCa,     Missing,        WithRoot,                File(Missing)),
            (VerifyCa,     RelativeSystem, WithRoot,                File(RelativeSystem)),
            (VerifyCa,     System,         WithRoot,                File(System)),

            (VerifyFull,   Unset,          Unknown,                 SystemStore),
            (VerifyFull,   Unset,          WithoutRoot,             SystemStore),
            (VerifyFull,   Unset,          WithRoot,                SystemStore),
            (VerifyFull,   Empty,          WithRoot,                File(Empty)),
            (VerifyFull,   Valid,          WithoutRoot,             File(Valid)),
            (VerifyFull,   Missing,        WithRoot,                File(Missing)),
            (VerifyFull,   RelativeSystem, WithRoot,                File(RelativeSystem)),
            (VerifyFull,   System,         Unknown,                 File(System)),
            (VerifyFull,   System,         WithRoot,                File(System)),
        ];

        for (ssl, cert, home, want) in cases {
            let got = select(ssl, cert, home);
            assert!(
                want.matches(&got),
                "{ssl:?} {cert:?} {home:?}: want {want:?}, got {got:?}"
            );
        }
    }

    #[test]
    fn disable_uses_no_tls() {
        assert!(connector(&conn(SslMode::Disable, None)).unwrap().is_none());
    }

    #[test]
    fn connector_loads_the_root_certificate() {
        let root_cert = testdata("root.crt");
        for ssl in [SslMode::Require, SslMode::VerifyCa, SslMode::VerifyFull] {
            assert!(connector(&conn(ssl, Some(&root_cert))).unwrap().is_some());
        }
    }

    #[test]
    fn connector_fails_on_an_invalid_root_certificate() {
        let invalid = testdata("invalid.crt");
        for ssl in [SslMode::Require, SslMode::VerifyCa, SslMode::VerifyFull] {
            let err = connector(&conn(ssl, Some(&invalid))).err().unwrap();
            assert!(format!("{err:#}").contains("reading ssl_root_cert"));
        }
    }

    #[test]
    fn connector_fails_on_a_missing_root_certificate() {
        let missing = Some("/nonexistent/root.crt");
        for ssl in [SslMode::Require, SslMode::VerifyCa, SslMode::VerifyFull] {
            assert!(connector(&conn(ssl, missing)).is_err());
        }
    }
}
