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
/// TODO(DATASEC-348): support uuid columns.
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
        Type::BYTEA => {
            |_, raw| Ok(String::from_utf8_lossy(types::bytea_from_sql(raw)).into_owned())
        }
        Type::INET | Type::CIDR => |_, raw| Ok(types::inet_from_sql(raw)?.addr().to_string()),
        Type::MACADDR => |_, raw| {
            let bytes = types::macaddr_from_sql(raw)?;
            Ok(bytes.map(|b| format!("{b:02x}")).join(":"))
        },
        Type::JSON | Type::XML => |_, raw| Ok(types::text_from_sql(raw)?.to_string()),
        // `jsonb` is its version byte followed by the JSON text.
        Type::JSONB => |_, raw| match raw.split_first() {
            Some((1, json)) => Ok(types::text_from_sql(json)?.to_string()),
            _ => Err("unsupported jsonb version".into()),
        },
        _ if ty.name() == "hstore" => |_, raw| {
            let pairs: Vec<String> = types::hstore_from_sql(raw)?
                .map(|(key, value)| Ok(format!("{key}=>{}", value.unwrap_or("NULL"))))
                .collect()?;
            Ok(pairs.join(", "))
        },
        // String types, including extensions such as `citext` and `ltree`.
        _ if <String as FromSql>::accepts(ty) => |ty, raw| String::from_sql(ty, raw),
        _ => return None,
    };
    Some(to_text)
}

impl<'a> FromSql<'a> for TextCell {
    fn from_sql(ty: &Type, raw: &'a [u8]) -> Result<Self, BoxError> {
        let text = match ty.kind() {
            Kind::Array(member) => array_text(member, raw)?,
            _ => to_text_fn(ty).ok_or("unsupported type")?(ty, raw)?,
        };
        Ok(Self(text))
    }

    fn accepts(ty: &Type) -> bool {
        match ty.kind() {
            Kind::Array(member) => Self::accepts(member),
            _ => to_text_fn(ty).is_some(),
        }
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

#[cfg(test)]
mod tests {
    use std::net::{IpAddr, Ipv6Addr};

    use postgres::types::private::BytesMut;
    use postgres::types::{FromSql, Kind, ToSql, Type};

    use super::TextCell;

    fn text(ty: Type, raw: &[u8]) -> String {
        TextCell::from_sql(&ty, raw).unwrap().0
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
        for ty in [
            Type::UUID,
            Type::UUID_ARRAY,
            Type::NUMERIC,
            Type::TIMESTAMP,
            Type::NUMERIC_ARRAY,
            extension("geometry"),
        ] {
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
        assert_eq!(
            text(Type::JSONB, b"\x01{\"email\": \"alice@corp.io\"}"),
            r#"{"email": "alice@corp.io"}"#
        );
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
        assert_eq!(text(extension("citext"), b"Alice@Corp.io"), "Alice@Corp.io");
        // `ltree` carries a version byte that must not end up in the text.
        assert_eq!(text(extension("ltree"), b"\x01top.science"), "top.science");
    }

    /// An extension type, known by name only.
    fn extension(name: &str) -> Type {
        Type::new(name.to_string(), 90000, Kind::Simple, "public".to_string())
    }

    #[test]
    fn converts_network_addresses_to_text() {
        let host: IpAddr = "192.168.0.1".parse().unwrap();
        assert_eq!(text(Type::INET, &encode(&host, &Type::INET)), "192.168.0.1");

        // cidr `2001:db8::/32`: family, netmask bits, is_cidr, address length, address.
        let mut cidr = vec![3, 32, 1, 16];
        cidr.extend("2001:db8::".parse::<Ipv6Addr>().unwrap().octets());
        assert_eq!(text(Type::CIDR, &cidr), "2001:db8::");
    }

    #[test]
    fn converts_jsonb_to_text() {
        // `jsonb` is a version byte (1) followed by the JSON text.
        let jsonb = |json: &str| [&[1u8], json.as_bytes()].concat();

        let profile = r#"{"contact": {"emails": ["alice@corp.io"]}, "age": 42}"#;
        assert_eq!(text(Type::JSONB, &jsonb(profile)), profile);

        // `jsonb[]`: header (1 dimension, has NULL, element oid), dimension, elements.
        let mut array = Vec::new();
        for word in [1, 1, Type::JSONB.oid() as i32, 2, 1] {
            array.extend(word.to_be_bytes());
        }
        let email = jsonb(r#""bob@corp.io""#);
        array.extend((email.len() as i32).to_be_bytes());
        array.extend(&email);
        array.extend((-1i32).to_be_bytes());
        assert_eq!(text(Type::JSONB_ARRAY, &array), r#"{"bob@corp.io",NULL}"#);

        // Only the version byte: empty JSON text, which the scanner just skips.
        assert_eq!(text(Type::JSONB, &[1]), "");
    }

    #[test]
    fn converts_hstore_to_text() {
        // Pair count, then each key and value as a length (-1 for NULL) and its bytes.
        let mut raw = 2i32.to_be_bytes().to_vec();
        for item in [Some("email"), Some("alice@corp.io"), Some("phone"), None] {
            match item {
                Some(item) => {
                    raw.extend((item.len() as i32).to_be_bytes());
                    raw.extend(item.as_bytes());
                }
                None => raw.extend((-1i32).to_be_bytes()),
            }
        }
        assert_eq!(
            text(extension("hstore"), &raw),
            "email=>alice@corp.io, phone=>NULL"
        );
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

    /// Binary array of `dimensions` lengths holding `items` (`None` is NULL).
    fn array(ty: &Type, dimensions: &[i32], items: &[Option<&[u8]>]) -> Vec<u8> {
        let mut raw = Vec::new();
        raw.extend((dimensions.len() as i32).to_be_bytes());
        raw.extend(i32::from(items.contains(&None)).to_be_bytes());
        raw.extend(ty.oid().to_be_bytes());
        for len in dimensions {
            raw.extend(len.to_be_bytes());
            raw.extend(1i32.to_be_bytes());
        }
        for item in items {
            match item {
                Some(value) => {
                    raw.extend((value.len() as i32).to_be_bytes());
                    raw.extend(*value);
                }
                None => raw.extend((-1i32).to_be_bytes()),
            }
        }
        raw
    }

    #[test]
    fn converts_multidimensional_arrays_to_text() {
        let emails = array(
            &Type::TEXT,
            &[2, 2],
            &[
                Some(b"alice@corp.io"),
                Some(b"bob@corp.io"),
                None,
                Some(b"carol@corp.io"),
            ],
        );
        assert_eq!(
            text(Type::TEXT_ARRAY, &emails),
            "{alice@corp.io,bob@corp.io,NULL,carol@corp.io}"
        );

        let one = 1i32.to_be_bytes();
        let two = 2i32.to_be_bytes();
        let ints = array(
            &Type::INT4,
            &[2, 1, 2],
            &[Some(&one), Some(&two), Some(&two), Some(&one)],
        );
        assert_eq!(text(Type::INT4_ARRAY, &ints), "{1,2,2,1}");
    }

    #[test]
    fn rejects_truncated_arrays() {
        let raw = array(&Type::TEXT, &[1], &[Some(b"alice@corp.io")]);
        assert!(TextCell::from_sql(&Type::TEXT_ARRAY, &raw[..raw.len() - 1]).is_err());
        assert!(TextCell::from_sql(&Type::TEXT_ARRAY, &raw[..6]).is_err());
    }

    #[test]
    fn rejects_unknown_jsonb_version() {
        assert!(TextCell::from_sql(&Type::JSONB, b"\x02{}").is_err());
        assert!(TextCell::from_sql(&Type::JSONB, b"").is_err());
    }
}
