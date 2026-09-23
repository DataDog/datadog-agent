//! MySQL scan engine.

use anyhow::{Context, Result, bail};
use mysql::consts::{ColumnFlags, ColumnType};
use mysql::prelude::Queryable;
use mysql::{AccessMode, ClientIdentity, Conn, OptsBuilder, Row, SslOpts, TxOpts, Value};
use openssl::pkcs12::Pkcs12;
use openssl::pkey::PKey;
use openssl::stack::Stack;
use openssl::x509::X509;
use std::io::Write;
use std::path::PathBuf;
use tempfile::NamedTempFile;

use crate::backend::{ScanData, ScanEngine, ScannedColumn};
use crate::config::{MySqlSsl, SubTask};

pub struct MySqlEngine;
pub const ENGINE: MySqlEngine = MySqlEngine;

impl ScanEngine for MySqlEngine {
    fn name(&self) -> &'static str {
        "mysql"
    }

    fn fetch_data(&self, sub_task: &SubTask) -> Result<ScanData> {
        // WARNING: do not modify the transaction/`prep` calls unless you know
        // what you are doing. Scanning stays read-only thanks to two properties
        // held together:
        //   - a read-only transaction (`AccessMode::ReadOnly`), which makes the
        //     server reject any write;
        //   - a single prepared statement via `prep`, which rejects
        //     multi-statement input and so blocks piggy-backed writes.
        // Weakening either one removes the guarantee that scanning stays read-only.
        let mut conn = connect(sub_task)?;
        set_query_timeout(&mut conn, sub_task.timeout)?;

        let mut tx = conn
            .start_transaction(TxOpts::default().set_access_mode(Some(AccessMode::ReadOnly)))
            .context("starting read-only mysql transaction")?;
        let stmt = tx
            .prep(sub_task.query.as_str())
            .context("preparing mysql query")?;
        let (indices, scanned_columns) = columns_from_stmt(&stmt.columns());
        let rows: Vec<Row> = tx.exec(&stmt, ()).context("running mysql query")?;
        tx.rollback()
            .context("rolling back mysql scan transaction")?;

        Ok(rows_to_scan_data(indices, scanned_columns, rows))
    }
}

fn connect(sub_task: &SubTask) -> Result<Conn> {
    let conn = &sub_task.connection;
    if conn.host.is_empty() && conn.sock.is_none() {
        bail!("mysql connection host or socket is required");
    }

    let mut opts = OptsBuilder::default()
        .ip_or_hostname((!conn.host.is_empty()).then(|| conn.host.clone()))
        .tcp_port(conn.port.unwrap_or(3306))
        .socket(conn.sock.clone())
        .prefer_socket(conn.sock.is_some())
        .user((!conn.username.is_empty()).then(|| conn.username.clone()))
        .pass((!conn.password.is_empty()).then(|| conn.password.clone()))
        .db_name((!conn.dbname.is_empty()).then(|| conn.dbname.clone()))
        .tcp_connect_timeout(Some(sub_task.timeout))
        .read_timeout(Some(sub_task.timeout))
        .write_timeout(Some(sub_task.timeout));

    let identity_file = if let Some(ssl) = conn.ssl.mysql() {
        let (ssl_opts, identity_file) = mysql_ssl_opts(ssl)?;
        opts = opts.ssl_opts(Some(ssl_opts));
        identity_file
    } else {
        None
    };

    let result = Conn::new(opts).context("connecting to mysql");
    drop(identity_file);
    result
}

fn set_query_timeout(conn: &mut Conn, timeout: std::time::Duration) -> Result<()> {
    let version: Option<String> = conn
        .query_first("SELECT VERSION()")
        .context("reading mysql server version")?;
    let statement = if version
        .as_deref()
        .is_some_and(|version| version.contains("MariaDB"))
    {
        format!("SET SESSION max_statement_time = {}", timeout.as_secs_f64())
    } else {
        format!("SET SESSION max_execution_time = {}", timeout.as_millis())
    };

    conn.query_drop(statement)
        .context("setting mysql query timeout")
}

/// Builds TLS options with the same verification defaults as PyMySQL:
/// a configured CA enables certificate and hostname verification; without a
/// CA, TLS is encrypted but the server certificate is not verified.
fn mysql_ssl_opts(ssl: &MySqlSsl) -> Result<(SslOpts, Option<NamedTempFile>)> {
    let (identity, identity_file) = match (&ssl.cert, &ssl.key) {
        (Some(cert), Some(key)) => {
            let file = pem_identity_file(cert, key)?;
            (
                Some(ClientIdentity::new(file.path().to_path_buf()).with_password("")),
                Some(file),
            )
        }
        (None, None) => (None, None),
        _ => bail!("mysql ssl cert and key must be set together"),
    };
    let has_ca = ssl.ca.is_some();
    Ok((
        SslOpts::default()
            .with_root_cert_path(ssl.ca.as_ref().map(PathBuf::from))
            .with_client_identity(identity)
            .with_danger_accept_invalid_certs(!has_ca)
            .with_danger_skip_domain_validation(!has_ca || !ssl.check_hostname()),
        identity_file,
    ))
}

/// The mysql crate's native-TLS backend consumes PKCS#12 identities while the
/// MySQL integration exposes PEM certificate and key paths. Convert them in a
/// private temporary file kept alive until the TLS connection is established.
fn pem_identity_file(cert_path: &str, key_path: &str) -> Result<NamedTempFile> {
    let cert_pem =
        std::fs::read(cert_path).with_context(|| format!("reading mysql ssl cert {cert_path}"))?;
    let key_pem =
        std::fs::read(key_path).with_context(|| format!("reading mysql ssl key {key_path}"))?;
    let mut certs = X509::stack_from_pem(&cert_pem)
        .with_context(|| format!("parsing mysql ssl cert {cert_path}"))?;
    if certs.is_empty() {
        bail!("mysql ssl cert {cert_path} contains no certificates");
    }
    let cert = certs.remove(0);
    let key = PKey::private_key_from_pem(&key_pem)
        .with_context(|| format!("parsing mysql ssl key {key_path}"))?;

    let mut builder = Pkcs12::builder();
    builder.name("datadog-agent").pkey(&key).cert(&cert);
    if !certs.is_empty() {
        let mut chain = Stack::new().context("creating mysql ssl certificate chain")?;
        for cert in certs {
            chain
                .push(cert)
                .context("building mysql ssl certificate chain")?;
        }
        builder.ca(chain);
    }
    let identity = builder
        .build2("")
        .context("building mysql ssl client identity")?
        .to_der()
        .context("encoding mysql ssl client identity")?;

    let mut file = NamedTempFile::new().context("creating mysql ssl client identity file")?;
    file.write_all(&identity)
        .context("writing mysql ssl client identity file")?;
    file.flush()
        .context("flushing mysql ssl client identity file")?;
    Ok(file)
}

fn columns_from_stmt(columns: &[mysql::Column]) -> (Vec<usize>, Vec<ScannedColumn>) {
    let mut indices = Vec::new();
    let mut scanned_columns = Vec::new();
    for (index, column) in columns.iter().enumerate() {
        let Some(data_type) = mysql_data_type(column) else {
            continue;
        };
        indices.push(index);
        scanned_columns.push(ScannedColumn {
            name: column.name_str().into_owned(),
            data_type: data_type.to_string(),
        });
    }
    (indices, scanned_columns)
}

fn mysql_data_type(column: &mysql::Column) -> Option<&'static str> {
    use ColumnType::*;

    let column_type = column.column_type();
    if column_type != MYSQL_TYPE_JSON && column.flags().contains(ColumnFlags::BINARY_FLAG) {
        return None;
    }

    Some(match column_type {
        MYSQL_TYPE_VARCHAR => "varchar",
        MYSQL_TYPE_VAR_STRING => "var_string",
        MYSQL_TYPE_STRING => "string",
        MYSQL_TYPE_TINY_BLOB => "tinyblob",
        MYSQL_TYPE_MEDIUM_BLOB => "mediumblob",
        MYSQL_TYPE_LONG_BLOB => "longblob",
        MYSQL_TYPE_BLOB => "blob",
        MYSQL_TYPE_JSON => "json",
        MYSQL_TYPE_ENUM => "enum",
        MYSQL_TYPE_SET => "set",
        _ => return None,
    })
}

fn rows_to_scan_data(
    indices: Vec<usize>,
    scanned_columns: Vec<ScannedColumn>,
    rows: Vec<Row>,
) -> ScanData {
    let rows = rows
        .into_iter()
        .map(|row| {
            let values = row.unwrap();
            indices
                .iter()
                .map(|&index| match &values[index] {
                    Value::NULL => None,
                    Value::Bytes(value) => Some(String::from_utf8_lossy(value).into_owned()),
                    _ => None,
                })
                .collect()
        })
        .collect();

    ScanData {
        scanned_columns,
        rows,
    }
}

#[cfg(test)]
mod tests {
    use super::{mysql_data_type, mysql_ssl_opts};
    use crate::config::MySqlSsl;
    use mysql::consts::{ColumnFlags, ColumnType};

    #[test]
    fn scans_mysql_text_but_not_binary_columns() {
        let text = mysql::Column::new(ColumnType::MYSQL_TYPE_VARCHAR);
        assert_eq!(mysql_data_type(&text), Some("varchar"));

        let binary =
            mysql::Column::new(ColumnType::MYSQL_TYPE_VARCHAR).with_flags(ColumnFlags::BINARY_FLAG);
        assert_eq!(mysql_data_type(&binary), None);

        let number = mysql::Column::new(ColumnType::MYSQL_TYPE_LONG);
        assert_eq!(mysql_data_type(&number), None);
    }

    #[test]
    fn requires_client_certificate_and_key_together() {
        let ssl = MySqlSsl {
            cert: Some("/cert.pem".to_string()),
            ..Default::default()
        };
        assert!(mysql_ssl_opts(&ssl).is_err());
    }

    #[test]
    fn matches_pymysql_tls_verification_defaults() {
        let (without_ca, _) = mysql_ssl_opts(&MySqlSsl::default()).unwrap();
        assert!(without_ca.accept_invalid_certs());
        assert!(without_ca.skip_domain_validation());

        let (with_ca, _) = mysql_ssl_opts(&MySqlSsl {
            ca: Some("/ca.pem".to_string()),
            ..Default::default()
        })
        .unwrap();
        assert!(!with_ca.accept_invalid_certs());
        assert!(!with_ca.skip_domain_validation());
    }
}
