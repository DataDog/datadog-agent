//! Converts postgres cells to text for scanning, like a `::text` cast.

use std::error::Error;

use postgres::fallible_iterator::FallibleIterator;
use postgres::types::{FromSql, Kind, Type};
use postgres_protocol::types;

type BoxError = Box<dyn Error + Sync + Send>;
type ToText = fn(&Type, &[u8]) -> Result<String, BoxError>;

/// A cell converted to text through [`to_text_fn`].
pub(super) struct TextCell(pub String);

/// Text conversion per type. Add a type here to scan it.
fn to_text_fn(ty: &Type) -> Option<ToText> {
    let to_text: ToText = match *ty {
        Type::BOOL => |_, raw| Ok(types::bool_from_sql(raw)?.to_string()),
        Type::CHAR => |_, raw| Ok(char::from(types::char_from_sql(raw)? as u8).to_string()),
        Type::INT2 => |_, raw| Ok(types::int2_from_sql(raw)?.to_string()),
        Type::INT4 => |_, raw| Ok(types::int4_from_sql(raw)?.to_string()),
        Type::INT8 => |_, raw| Ok(types::int8_from_sql(raw)?.to_string()),
        Type::OID => |_, raw| Ok(types::oid_from_sql(raw)?.to_string()),
        Type::FLOAT4 => |_, raw| Ok(types::float4_from_sql(raw)?.to_string()),
        Type::FLOAT8 => |_, raw| Ok(types::float8_from_sql(raw)?.to_string()),
        Type::BYTEA => |_, raw| Ok(bytea_text(raw)),
        Type::INET | Type::CIDR => |_, raw| Ok(types::inet_from_sql(raw)?.addr().to_string()),
        Type::MACADDR => |_, raw| macaddr_text(raw),
        Type::UUID => |_, raw| uuid_text(raw),
        Type::JSON | Type::XML => |_, raw| Ok(types::text_from_sql(raw)?.to_string()),
        Type::JSONB => |_, raw| jsonb_text(raw),
        _ if ty.name() == "hstore" => |_, raw| hstore_text(raw),
        // String types, including extensions such as `citext` and `ltree`.
        _ if <String as FromSql>::accepts(ty) => |ty, raw| String::from_sql(ty, raw),
        // Reported as scanned columns but read as empty text, which is not scanned: they hold
        // no sensitive data.
        Type::DATE | Type::TIME | Type::TIMESTAMP | Type::TIMESTAMPTZ | Type::INTERVAL => {
            |_, _| Ok(String::new())
        }
        _ => return None,
    };
    Some(to_text)
}

impl<'a> FromSql<'a> for TextCell {
    fn from_sql(ty: &Type, raw: &'a [u8]) -> Result<Self, BoxError> {
        if let Kind::Array(member) = ty.kind() {
            return Ok(Self(array_text(member, raw)?));
        }
        let to_text = to_text_fn(ty).ok_or("unsupported type")?;
        Ok(Self(to_text(ty, raw)?))
    }

    fn accepts(ty: &Type) -> bool {
        if let Kind::Array(member) = ty.kind() {
            return Self::accepts(member);
        }
        to_text_fn(ty).is_some()
    }
}

/// Formats an array of any dimension as `{a,NULL,b}`, flattening its elements.
fn array_text(member: &Type, raw: &[u8]) -> Result<String, BoxError> {
    let items: Vec<String> = types::array_from_sql(raw)?
        .values()
        .map(|value| match value {
            Some(value) => Ok(TextCell::from_sql(member, value)?.0),
            None => Ok("NULL".to_string()),
        })
        .collect()?;
    Ok(format!("{{{}}}", items.join(",")))
}

/// Decodes `bytea` as UTF-8, replacing invalid sequences.
fn bytea_text(raw: &[u8]) -> String {
    String::from_utf8_lossy(types::bytea_from_sql(raw)).into_owned()
}

/// Formats a `macaddr` as `08:00:2b:01:02:03`.
fn macaddr_text(raw: &[u8]) -> Result<String, BoxError> {
    let bytes = types::macaddr_from_sql(raw)?;
    Ok(bytes.map(|b| format!("{b:02x}")).join(":"))
}

/// Formats a `uuid` as `00112233-4455-6677-8899-aabbccddeeff`.
fn uuid_text(raw: &[u8]) -> Result<String, BoxError> {
    let mut uuid = String::new();
    for (i, b) in types::uuid_from_sql(raw)?.iter().enumerate() {
        if matches!(i, 4 | 6 | 8 | 10) {
            uuid.push('-');
        }
        uuid.push_str(&format!("{b:02x}"));
    }
    Ok(uuid)
}

/// Reads the JSON text of a `jsonb`, which is its version byte followed by the JSON text.
fn jsonb_text(raw: &[u8]) -> Result<String, BoxError> {
    match raw.split_first() {
        Some((1, json)) => Ok(types::text_from_sql(json)?.to_string()),
        _ => Err("unsupported jsonb version".into()),
    }
}

/// Formats an `hstore` as `key=>value, key=>NULL`.
fn hstore_text(raw: &[u8]) -> Result<String, BoxError> {
    let pairs: Vec<String> = types::hstore_from_sql(raw)?
        .map(|(key, value)| Ok(format!("{key}=>{}", value.unwrap_or("NULL"))))
        .collect()?;
    Ok(pairs.join(", "))
}

#[cfg(test)]
mod tests {
    use std::net::{IpAddr, Ipv6Addr};
    use std::time::SystemTime;

    use postgres::types::private::BytesMut;
    use postgres::types::{FromSql, Kind, ToSql, Type};

    use super::TextCell;

    fn text(ty: Type, raw: &[u8]) -> String {
        TextCell::from_sql(&ty, raw).unwrap().0
    }

    /// An extension type, known by name only.
    fn extension(name: &str) -> Type {
        Type::new(name.to_string(), 90000, Kind::Simple, "public".to_string())
    }

    /// Encodes `value` in the postgres binary format of `ty`.
    fn encode(value: &(dyn ToSql + Sync), ty: &Type) -> BytesMut {
        let mut raw = BytesMut::new();
        value.to_sql_checked(ty, &mut raw).unwrap();
        raw
    }

    #[test]
    fn accepts_types_convertible_to_text() {
        for ty in [
            Type::TEXT,
            Type::VARCHAR,
            Type::BPCHAR,
            Type::NAME,
            Type::BOOL,
            Type::INT2,
            Type::INT4,
            Type::INT8,
            Type::OID,
            Type::FLOAT4,
            Type::FLOAT8,
            Type::CHAR,
            Type::BYTEA,
            Type::INET,
            Type::CIDR,
            Type::MACADDR,
            Type::UUID,
            Type::UUID_ARRAY,
            Type::DATE,
            Type::TIME,
            Type::TIMESTAMP,
            Type::TIMESTAMPTZ,
            Type::TIMESTAMP_ARRAY,
            Type::INTERVAL,
            Type::JSON,
            Type::JSONB,
            Type::XML,
            extension("hstore"),
            extension("citext"),
            extension("ltree"),
            Type::TEXT_ARRAY,
            Type::INT4_ARRAY,
            Type::JSONB_ARRAY,
            Type::INET_ARRAY,
        ] {
            assert!(TextCell::accepts(&ty), "{ty} should be accepted");
        }
        for ty in [Type::NUMERIC, Type::NUMERIC_ARRAY, extension("geometry")] {
            assert!(!TextCell::accepts(&ty), "{ty} should not be accepted");
        }
    }

    #[test]
    fn converts_cells_to_text() {
        assert_eq!(text(Type::TEXT, b"alice@corp.io"), "alice@corp.io");
        assert_eq!(text(Type::BOOL, &[1]), "true");
        assert_eq!(text(Type::INT2, &(-7i16).to_be_bytes()), "-7");
        assert_eq!(text(Type::INT4, &123456i32.to_be_bytes()), "123456");
        assert_eq!(
            text(Type::INT8, &4242424242424242i64.to_be_bytes()),
            "4242424242424242"
        );
        assert_eq!(text(Type::OID, &42u32.to_be_bytes()), "42");
        assert_eq!(text(Type::FLOAT4, &1.5f32.to_be_bytes()), "1.5");
        assert_eq!(text(Type::FLOAT8, &(-0.25f64).to_be_bytes()), "-0.25");
        assert_eq!(
            text(Type::JSON, br#"{"email": "alice@corp.io"}"#),
            r#"{"email": "alice@corp.io"}"#
        );
        // `jsonb` is a version byte (1) followed by the JSON text.
        assert_eq!(
            text(Type::JSONB, b"\x01{\"email\": \"alice@corp.io\"}"),
            r#"{"email": "alice@corp.io"}"#
        );
        assert_eq!(text(Type::JSONB, &[1]), "");
        assert_eq!(
            text(Type::XML, b"<email>alice@corp.io</email>"),
            "<email>alice@corp.io</email>"
        );
        assert_eq!(text(Type::CHAR, b"a"), "a");
        assert_eq!(
            text(Type::BYTEA, b"alice@corp.io\xff"),
            "alice@corp.io\u{fffd}"
        );
        assert_eq!(
            text(Type::MACADDR, &[0x08, 0x00, 0x2b, 0x01, 0x02, 0x03]),
            "08:00:2b:01:02:03"
        );
        let uuid: Vec<u8> = (0..16).map(|b| b * 0x11).collect();
        assert_eq!(
            text(Type::UUID, &uuid),
            "00112233-4455-6677-8899-aabbccddeeff"
        );
        assert!(TextCell::from_sql(&Type::UUID, &uuid[..15]).is_err());
        let host: IpAddr = "192.168.0.1".parse().unwrap();
        assert_eq!(text(Type::INET, &encode(&host, &Type::INET)), "192.168.0.1");
        // cidr `2001:db8::/32`: family, netmask bits, is_cidr, address length, address.
        let mut cidr = vec![3, 32, 1, 16];
        cidr.extend("2001:db8::".parse::<Ipv6Addr>().unwrap().octets());
        assert_eq!(text(Type::CIDR, &cidr), "2001:db8::");
        // hstore: pair count, then each key and value as a length (-1 for NULL) and its bytes.
        let mut hstore = 2i32.to_be_bytes().to_vec();
        for item in [Some("email"), Some("alice@corp.io"), Some("phone"), None] {
            match item {
                Some(item) => {
                    hstore.extend((item.len() as i32).to_be_bytes());
                    hstore.extend(item.as_bytes());
                }
                None => hstore.extend((-1i32).to_be_bytes()),
            }
        }
        assert_eq!(
            text(extension("hstore"), &hstore),
            "email=>alice@corp.io, phone=>NULL"
        );
        assert_eq!(text(extension("citext"), b"Alice@Corp.io"), "Alice@Corp.io");
        // `ltree` carries a version byte that must not end up in the text.
        assert_eq!(text(extension("ltree"), b"\x01top.science"), "top.science");
        // Dates, times and intervals are reported but read as empty text.
        let now = SystemTime::now();
        assert_eq!(text(Type::TIMESTAMP, &encode(&now, &Type::TIMESTAMP)), "");
        assert_eq!(
            text(Type::TIMESTAMPTZ, &encode(&now, &Type::TIMESTAMPTZ)),
            ""
        );
        assert_eq!(text(Type::DATE, &0i32.to_be_bytes()), "");
        assert_eq!(text(Type::TIME, &0i64.to_be_bytes()), "");
        assert_eq!(text(Type::INTERVAL, &[0; 16]), "");
    }

    #[test]
    fn converts_arrays_to_text() {
        let emails = encode(
            &vec![Some("alice@corp.io"), None, Some("bob@corp.io")],
            &Type::TEXT_ARRAY,
        );
        assert_eq!(
            text(Type::TEXT_ARRAY, &emails),
            "{alice@corp.io,NULL,bob@corp.io}"
        );

        let ints = encode(&vec![1i32, -2], &Type::INT4_ARRAY);
        assert_eq!(text(Type::INT4_ARRAY, &ints), "{1,-2}");

        let empty = encode(&Vec::<&str>::new(), &Type::VARCHAR_ARRAY);
        assert_eq!(text(Type::VARCHAR_ARRAY, &empty), "{}");
    }

    // TODO(DATASEC-349): add multidimensional array tests.

    #[test]
    fn rejects_truncated_arrays() {
        let raw = encode(&vec!["alice@corp.io"], &Type::TEXT_ARRAY);
        assert!(TextCell::from_sql(&Type::TEXT_ARRAY, &raw[..raw.len() - 1]).is_err());
        assert!(TextCell::from_sql(&Type::TEXT_ARRAY, &raw[..6]).is_err());
    }

    #[test]
    fn rejects_unsupported_types() {
        assert!(TextCell::from_sql(&Type::NUMERIC, &[0; 8]).is_err());
    }

    #[test]
    fn rejects_unknown_jsonb_version() {
        assert!(TextCell::from_sql(&Type::JSONB, b"\x02{}").is_err());
        assert!(TextCell::from_sql(&Type::JSONB, b"").is_err());
    }
}
