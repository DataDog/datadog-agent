use std::path::PathBuf;

use anyhow::{Context, Result, bail};
use openssl::pkey::PKey;
use openssl::ssl::{SslConnector, SslConnectorBuilder, SslMethod, SslVerifyMode};
use openssl::x509::store::X509StoreBuilder;
use postgres_openssl::MakeTlsConnector;

use crate::config::{Connection, SslMode};

/// libpq's default `sslrootcert`, relative to the home directory.
const DEFAULT_ROOT_CERT: &str = ".postgresql/root.crt";

/// The magic `ssl_root_cert` value selecting the system CA store, like libpq's `sslrootcert=system`
const SYSTEM_ROOT_CERT: &str = "system";

#[derive(Debug, PartialEq, Eq)]
enum RootCert {
    System,
    File(PathBuf),
}

/// `~/.postgresql/root.crt`. The home directory is `$HOME`, else the passwd entry, like libpq.
fn default_root_cert() -> Option<PathBuf> {
    std::env::home_dir().map(|home| home.join(DEFAULT_ROOT_CERT))
}

/// CAs to verify the server certificate with, or `None` when it isn't verified.
///
/// Like libpq, `require` verifies the certificate chain (not the host name) when a root
/// certificate is provided. This is what the postgres integration documents for `ssl`, see
/// https://github.com/DataDog/integrations-core/blob/e5b2b9c9e1bcb2caa1a3de3a44a11bfa3678d6aa/postgres/assets/configuration/spec.yaml#L197-L198
///
/// The root certificate is selected like libpq's `sslrootcert`, including `require` not
/// verifying when the file doesn't exist, see
/// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNECT-SSLROOTCERT
fn verification_root_cert(conn: &Connection) -> Result<Option<RootCert>> {
    select_root_cert(conn, default_root_cert())
}

/// [`verification_root_cert`] given the default root certificate path.
fn select_root_cert(conn: &Connection, default: Option<PathBuf>) -> Result<Option<RootCert>> {
    // Like libpq and the integration, an empty value is unset.
    let ssl_root_cert = conn
        .ssl_root_cert
        .as_deref()
        .filter(|path| !path.is_empty());
    if ssl_root_cert == Some(SYSTEM_ROOT_CERT) && conn.ssl != SslMode::VerifyFull {
        bail!("ssl_root_cert `system` requires ssl `verify-full`, as weaker modes are insecure");
    }
    if matches!(
        conn.ssl,
        SslMode::Disable | SslMode::Allow | SslMode::Prefer
    ) {
        return Ok(None);
    }
    let path = match ssl_root_cert {
        Some(SYSTEM_ROOT_CERT) => return Ok(Some(RootCert::System)),
        Some(path) => Some(PathBuf::from(path)),
        None => default,
    };
    match path.as_ref().filter(|path| path.exists()) {
        Some(_) => Ok(path.map(RootCert::File)),
        None if conn.ssl == SslMode::Require => Ok(None),
        None => bail!(
            "root certificate file {} does not exist: set ssl_root_cert (`system` for the \
             system CA store) or use another ssl mode",
            path.map_or_else(
                || format!("~/{DEFAULT_ROOT_CERT}"),
                |p| p.display().to_string()
            )
        ),
    }
}

/// OpenSSL connector, or `None` for `disable` (`NoTls`).
pub fn connector(conn: &Connection) -> Result<Option<MakeTlsConnector>> {
    let root_cert = verification_root_cert(conn)?;
    if conn.ssl == SslMode::Disable {
        return Ok(None);
    }

    let mut builder =
        SslConnector::builder(SslMethod::tls()).context("creating the OpenSSL connector")?;

    match root_cert {
        // set_ca_file adds to the builder's system store; replace it so we trust only this CA.
        Some(RootCert::File(path)) => {
            builder.set_verify(SslVerifyMode::PEER);
            builder.set_cert_store(
                X509StoreBuilder::new()
                    .context("creating CA store")?
                    .build(),
            );
            builder
                .set_ca_file(&path)
                .with_context(|| format!("reading root certificate {}", path.display()))?;
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

    impl Home {
        fn default_root_cert(self) -> Option<PathBuf> {
            match self {
                Home::Unknown => None,
                Home::WithRoot => Some(PathBuf::from(testdata("root.crt"))),
                Home::WithoutRoot => Some(PathBuf::from("/nonexistent/.postgresql/root.crt")),
            }
        }
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
        /// An error containing this text.
        Error(&'static str),
    }

    /// The selected root certificate.
    #[derive(Debug)]
    enum Got {
        Unverified,
        SystemStore,
        File(String),
        Error(String),
    }

    impl Want {
        fn matches(&self, got: &Got) -> bool {
            match (self, got) {
                (Want::Unverified, Got::Unverified) | (Want::SystemStore, Got::SystemStore) => true,
                (Want::File(cert), Got::File(path)) => cert.value().as_deref() == Some(path),
                (Want::Error(text), Got::Error(err)) => err.contains(text),
                _ => false,
            }
        }
    }

    fn select(ssl: SslMode, cert: Cert, home: Home) -> Got {
        match select_root_cert(
            &conn(ssl, cert.value().as_deref()),
            home.default_root_cert(),
        ) {
            Err(err) => Got::Error(err.to_string()),
            Ok(None) => Got::Unverified,
            Ok(Some(RootCert::System)) => Got::SystemStore,
            Ok(Some(RootCert::File(path))) => Got::File(path.display().to_string()),
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
            (Disable,      System,         Unknown,                 Error("verify-full")),
            (Allow,        Unset,          WithRoot,                Unverified),
            (Allow,        Valid,          WithRoot,                Unverified),
            (Allow,        Missing,        WithRoot,                Unverified),
            (Allow,        System,         Unknown,                 Error("verify-full")),
            (Prefer,       Unset,          WithRoot,                Unverified),
            (Prefer,       Valid,          WithRoot,                Unverified),
            (Prefer,       Missing,        WithRoot,                Unverified),
            (Prefer,       System,         Unknown,                 Error("verify-full")),

            (Require,      Unset,          Unknown,                 Unverified),
            (Require,      Unset,          WithoutRoot,             Unverified),
            (Require,      Unset,          WithRoot,                File(Valid)),
            (Require,      Empty,          WithRoot,                File(Valid)),
            (Require,      Valid,          WithoutRoot,             File(Valid)),
            (Require,      Missing,        WithRoot,                Unverified),
            (Require,      RelativeSystem, WithRoot,                Unverified),
            (Require,      System,         WithRoot,                Error("verify-full")),

            (VerifyCa,     Unset,          Unknown,                 Error("~/.postgresql/root.crt")),
            (VerifyCa,     Unset,          WithoutRoot,             Error("/nonexistent/.postgresql/root.crt")),
            (VerifyCa,     Unset,          WithRoot,                File(Valid)),
            (VerifyCa,     Empty,          WithRoot,                File(Valid)),
            (VerifyCa,     Valid,          WithoutRoot,             File(Valid)),
            (VerifyCa,     Missing,        WithRoot,                Error("/nonexistent/root.crt")),
            (VerifyCa,     RelativeSystem, WithRoot,                Error("./system")),
            (VerifyCa,     System,         WithRoot,                Error("verify-full")),

            (VerifyFull,   Unset,          Unknown,                 Error("~/.postgresql/root.crt")),
            (VerifyFull,   Unset,          WithoutRoot,             Error("/nonexistent/.postgresql/root.crt")),
            (VerifyFull,   Unset,          WithRoot,                File(Valid)),
            (VerifyFull,   Empty,          WithRoot,                File(Valid)),
            (VerifyFull,   Valid,          WithoutRoot,             File(Valid)),
            (VerifyFull,   Missing,        WithRoot,                Error("/nonexistent/root.crt")),
            (VerifyFull,   RelativeSystem, WithRoot,                Error("./system")),
            (VerifyFull,   System,         Unknown,                 SystemStore),
            (VerifyFull,   System,         WithRoot,                SystemStore),
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
            assert!(format!("{err:#}").contains("reading root certificate"));
        }
    }

    #[test]
    fn connector_fails_on_a_missing_root_certificate() {
        let missing = Some("/nonexistent/root.crt");
        for ssl in [SslMode::VerifyCa, SslMode::VerifyFull] {
            assert!(connector(&conn(ssl, missing)).is_err());
        }
    }

    #[test]
    fn disable_rejects_the_system_root_certificate() {
        // Even though `disable` doesn't use TLS.
        assert!(connector(&conn(SslMode::Disable, Some("system"))).is_err());
    }

    #[test]
    fn verify_full_loads_the_system_root_certificate() {
        let ssl_root_cert = Some("system");
        assert!(
            connector(&conn(SslMode::VerifyFull, ssl_root_cert))
                .unwrap()
                .is_some()
        );
    }
}
