use std::fmt;

use postgres::error::SqlState;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PostgresError {
    Server(SqlState),
    Client(String),
}

impl fmt::Display for PostgresError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Server(state) => write!(
                f,
                "postgres server error: {} (SQLSTATE {})",
                describe(state),
                state.code()
            ),
            Self::Client(message) => write!(f, "postgres client error: {message}"),
        }
    }
}

// No `source()`: must not chain back to the original error.
impl std::error::Error for PostgresError {}

impl From<postgres::Error> for PostgresError {
    fn from(err: postgres::Error) -> Self {
        match err.code() {
            Some(state) => Self::Server(state.clone()),
            None => Self::Client(err.to_string()),
        }
    }
}

fn describe(state: &SqlState) -> &'static str {
    const KNOWN: &[(SqlState, &str)] = &[
        (
            SqlState::INSUFFICIENT_PRIVILEGE,
            "permission denied, the user lacks the required privileges",
        ),
        (SqlState::INVALID_CATALOG_NAME, "database does not exist"),
        (SqlState::INVALID_SCHEMA_NAME, "schema does not exist"),
        (SqlState::UNDEFINED_TABLE, "table does not exist"),
        (SqlState::UNDEFINED_COLUMN, "column does not exist"),
        (
            SqlState::QUERY_CANCELED,
            "query canceled (statement timeout)",
        ),
        (
            SqlState::READ_ONLY_SQL_TRANSACTION,
            "write attempted in a read-only transaction",
        ),
        (
            SqlState::ADMIN_SHUTDOWN,
            "server is not accepting connections",
        ),
        (
            SqlState::CRASH_SHUTDOWN,
            "server is not accepting connections",
        ),
        (
            SqlState::CANNOT_CONNECT_NOW,
            "server is not accepting connections",
        ),
    ];
    if let Some((_, label)) = KNOWN.iter().find(|(known, _)| known == state) {
        return label;
    }
    // SQLSTATE class: https://www.postgresql.org/docs/current/errcodes-appendix.html
    match state.code().get(..2) {
        Some("28") => "authentication failed (check username, password and pg_hba)",
        Some("08") => "connection failure",
        Some("42") => "invalid query",
        Some("53") => "insufficient resources (e.g. too many connections)",
        _ => "",
    }
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::thread;

    use openssl::ssl::{SslConnector, SslMethod};
    use postgres::config::SslMode;
    use postgres::error::SqlState;
    use postgres_openssl::MakeTlsConnector;

    use super::PostgresError;

    #[test]
    fn server_errors_keep_only_sqlstate() {
        // Cast failure, used to leak `invalid input syntax for type integer: "<value>"`.
        assert_eq!(
            PostgresError::Server(SqlState::INVALID_TEXT_REPRESENTATION).to_string(),
            "postgres server error:  (SQLSTATE 22P02)"
        );
        assert_eq!(
            PostgresError::Server(SqlState::INSUFFICIENT_PRIVILEGE).to_string(),
            "postgres server error: permission denied, the user lacks the required privileges (SQLSTATE 42501)"
        );
    }

    #[test]
    fn client_timeout_error() {
        assert_eq!(
            PostgresError::from(postgres::Error::__private_api_timeout()).to_string(),
            "postgres client error: timeout waiting for server"
        );
    }

    #[test]
    fn client_tls_error_drops_cause() {
        // Fake server declining TLS.
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            let mut ssl_request = [0; 8];
            stream.read_exact(&mut ssl_request).unwrap();
            stream.write_all(b"N").unwrap();
        });
        let tls = MakeTlsConnector::new(SslConnector::builder(SslMethod::tls()).unwrap().build());
        let err = postgres::Config::new()
            .host("127.0.0.1")
            .port(port)
            .user("user")
            .ssl_mode(SslMode::Require)
            .connect(tls)
            .err()
            .expect("TLS should be refused");
        server.join().unwrap();

        let err = anyhow::Error::new(PostgresError::from(err)).context("connecting to postgres");
        assert_eq!(
            format!("{err:#}"),
            "connecting to postgres: postgres client error: error performing TLS handshake"
        );
    }
}
