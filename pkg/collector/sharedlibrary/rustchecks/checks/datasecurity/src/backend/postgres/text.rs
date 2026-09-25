//! Converts postgres cells to text for scanning, like a `::text` cast.

use std::error::Error;

use postgres::types::{FromSql, Type};
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
        Type::JSON | Type::XML => |_, raw| Ok(types::text_from_sql(raw)?.to_string()),
        // String types, including extensions such as `citext` and `ltree`.
        _ if <String as FromSql>::accepts(ty) => |ty, raw| String::from_sql(ty, raw),
        _ => return None,
    };
    Some(to_text)
}

impl<'a> FromSql<'a> for TextCell {
    fn from_sql(ty: &Type, raw: &'a [u8]) -> Result<Self, BoxError> {
        let to_text = to_text_fn(ty).ok_or("unsupported type")?;
        Ok(Self(to_text(ty, raw)?))
    }

    fn accepts(ty: &Type) -> bool {
        to_text_fn(ty).is_some()
    }
}

#[cfg(test)]
mod tests {
    use postgres::types::{FromSql, Kind, Type};

    use super::TextCell;

    fn text(ty: Type, raw: &[u8]) -> String {
        TextCell::from_sql(&ty, raw).unwrap().0
    }

    /// An extension type, known by name only.
    fn extension(name: &str) -> Type {
        Type::new(name.to_string(), 90000, Kind::Simple, "public".to_string())
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
            Type::JSON,
            Type::XML,
            extension("citext"),
            extension("ltree"),
        ] {
            assert!(TextCell::accepts(&ty), "{ty} should be accepted");
        }
        for ty in [
            Type::UUID,
            Type::NUMERIC,
            Type::TIMESTAMP,
            Type::JSONB,
            Type::TEXT_ARRAY,
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
            text(Type::XML, b"<email>alice@corp.io</email>"),
            "<email>alice@corp.io</email>"
        );
        assert_eq!(text(Type::CHAR, b"a"), "a");
        assert_eq!(text(extension("citext"), b"Alice@Corp.io"), "Alice@Corp.io");
        // `ltree` carries a version byte that must not end up in the text.
        assert_eq!(text(extension("ltree"), b"\x01top.science"), "top.science");
    }

    #[test]
    fn rejects_unsupported_types() {
        assert!(TextCell::from_sql(&Type::UUID, &[0; 16]).is_err());
    }
}
