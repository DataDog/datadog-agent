// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Adapted from the `vrl` crate (vectordotdev/vrl, crates.io "vrl" v0.33.1,
// src/stdlib/redact.rs), which is MPL-2.0 licensed. See ../../LICENSE-vrl
// for the full text. Upstream gates the hash redactor behind vrl's
// `enable_crypto_functions` feature (one of the all-or-nothing bundles that
// pulls in the `datadog`/`onig` feature chain we're avoiding); here the hash
// redactor is always compiled, using this crate's own sha2/sha3/base64/base16
// dependencies instead of vrl's internally re-exported/renamed crates.

use std::{
    borrow::Cow,
    convert::{TryFrom, TryInto},
    sync::LazyLock,
};

use vrl::prelude::*;

// https://www.oreilly.com/library/view/regular-expressions-cookbook/9781449327453/ch04s12.html
// (converted to non-lookaround version given `regex` does not support lookarounds)
// See also: https://www.ssa.gov/history/ssn/geocard.html
static US_SOCIAL_SECURITY_NUMBER: LazyLock<regex::Regex> = LazyLock::new(|| {
    regex::Regex::new(
        "(?x)                                                               # Ignore whitespace and comments in the regex expression.
    (?:00[1-9]|0[1-9][0-9]|[1-578][0-9]{2}|6[0-57-9][0-9]|66[0-57-9])-    # Area number: 001-899 except 666
    (?:0[1-9]|[1-9]0|[1-9][1-9])-                                         # Group number: 01-99
    (?:000[1-9]|00[1-9]0|0[1-9]00|[1-9]000|[1-9]{4})                      # Serial number: 0001-9999
    ").unwrap()
});

#[derive(Clone, Copy, Debug)]
pub struct Redact;

impl Function for Redact {
    fn identifier(&self) -> &'static str {
        "redact"
    }

    fn usage(&self) -> &'static str {
        indoc! {"
            Redact sensitive data in `value` such as:

            - US social security numbers
            - Other forms of personally identifiable information with custom patterns

            This can help achieve compliance by ensuring sensitive data does not leave your network.
        "}
    }

    fn category(&self) -> &'static str {
        "string"
    }

    fn return_kind(&self) -> u16 {
        kind::BYTES | kind::OBJECT | kind::ARRAY
    }

    fn parameters(&self) -> &'static [Parameter] {
        const PARAMETERS: &[Parameter] = &[
            Parameter::required(
                "value",
                kind::BYTES | kind::OBJECT | kind::ARRAY,
                "The value to redact sensitive data from.",
            ),
            Parameter::required(
                "filters",
                kind::ARRAY,
                "List of filters applied to `value`. Each filter is either a regular \
expression, the named filter `us_social_security_number`, or an object with a `type` \
key of `pattern` and a `patterns` array of regular expressions.",
            ),
            Parameter::optional(
                "redactor",
                kind::OBJECT | kind::BYTES,
                "What to replace redacted text with: `full` (default, \"[REDACTED]\"), \
`text` (a custom replacement string via the `replacement` key), `sha2`, or `sha3` \
(hash the redacted text; both accept optional `variant`/`encoding` keys).",
            ),
        ];
        PARAMETERS
    }

    fn examples(&self) -> &'static [Example] {
        &[]
    }

    fn compile(
        &self,
        state: &state::TypeState,
        _ctx: &mut FunctionCompileContext,
        arguments: ArgumentList,
    ) -> Compiled {
        let value = arguments.required("value");

        let filters = arguments
            .required_array("filters")?
            .into_iter()
            .map(|expr| {
                expr.resolve_constant(state)
                    .ok_or(function::Error::ExpectedStaticExpression {
                        keyword: "filters",
                        expr,
                    })
            })
            .map(|value| {
                value.and_then(|value| {
                    value
                        .clone()
                        .try_into()
                        .map_err(|error| function::Error::InvalidArgument {
                            keyword: "filters",
                            value,
                            error,
                        })
                })
            })
            .collect::<std::result::Result<Vec<Filter>, _>>()?;

        let redactor = arguments
            .optional_literal("redactor", state)?
            .map(|value| {
                value
                    .clone()
                    .try_into()
                    .map_err(|error| function::Error::InvalidArgument {
                        keyword: "redactor",
                        value,
                        error,
                    })
            })
            .transpose()?
            .unwrap_or(Redactor::Full);

        Ok(RedactFn {
            value,
            filters,
            redactor,
        }
        .as_expr())
    }
}

//-----------------------------------------------------------------------------

#[derive(Clone, Debug)]
struct RedactFn {
    value: Box<dyn Expression>,
    filters: Vec<Filter>,
    redactor: Redactor,
}

fn redact(value: Value, filters: &[Filter], redactor: &Redactor) -> Value {
    match value {
        Value::Bytes(bytes) => {
            let input = String::from_utf8_lossy(&bytes);
            let output = filters.iter().fold(input, |input, filter| {
                filter.redact(&input, redactor).into_owned().into()
            });
            Value::Bytes(output.into_owned().into())
        }
        Value::Array(values) => {
            let values = values
                .into_iter()
                .map(|value| redact(value, filters, redactor))
                .collect();
            Value::Array(values)
        }
        Value::Object(map) => {
            let map = map
                .into_iter()
                .map(|(key, value)| (key, redact(value, filters, redactor)))
                .collect();
            Value::Object(map)
        }
        _ => value,
    }
}

impl FunctionExpression for RedactFn {
    fn resolve(&self, ctx: &mut Context) -> Resolved {
        let value = self.value.resolve(ctx)?;
        let filters = &self.filters;
        let redactor = &self.redactor;

        Ok(redact(value, filters, redactor))
    }

    fn type_def(&self, state: &state::TypeState) -> TypeDef {
        self.value.type_def(state).infallible()
    }
}

//-----------------------------------------------------------------------------

/// The redaction filter to apply to the given value.
#[derive(Debug, Clone)]
enum Filter {
    Pattern(Vec<Pattern>),
    UsSocialSecurityNumber,
}

#[derive(Debug, Clone)]
enum Pattern {
    Regex(regex::Regex),
    String(String),
}

impl TryFrom<Value> for Filter {
    type Error = &'static str;

    fn try_from(value: Value) -> std::result::Result<Self, Self::Error> {
        match value {
            Value::Object(object) => {
                let r#type = match object
                    .get("type")
                    .ok_or("filters specified as objects must have type parameter")?
                {
                    Value::Bytes(bytes) => Ok(bytes.clone()),
                    _ => Err("type key in filters must be a string"),
                }?;

                match r#type.as_ref() {
                    b"us_social_security_number" => Ok(Filter::UsSocialSecurityNumber),
                    b"pattern" => {
                        let patterns = match object
                            .get("patterns")
                            .ok_or("pattern filter must have `patterns` specified")?
                        {
                            Value::Array(array) => Ok(array
                                .iter()
                                .map(|value| match value {
                                    Value::Regex(regex) => Ok(Pattern::Regex((**regex).clone())),
                                    Value::Bytes(bytes) => Ok(Pattern::String(
                                        String::from_utf8_lossy(bytes).into_owned(),
                                    )),
                                    _ => Err("`patterns` must be regular expressions"),
                                })
                                .collect::<std::result::Result<Vec<_>, _>>()?),
                            _ => Err("`patterns` must be array of regular expression literals"),
                        }?;
                        Ok(Filter::Pattern(patterns))
                    }
                    _ => Err("unknown filter name"),
                }
            }
            Value::Bytes(bytes) => match bytes.as_ref() {
                b"pattern" => Err("pattern cannot be used without arguments"),
                b"us_social_security_number" => Ok(Filter::UsSocialSecurityNumber),
                _ => Err("unknown filter name"),
            },
            Value::Regex(regex) => Ok(Filter::Pattern(vec![Pattern::Regex((*regex).clone())])),
            _ => Err("unknown literal for filter, must be a regex, filter name, or object"),
        }
    }
}

impl Filter {
    fn redact<'t>(&self, input: &'t str, redactor: &Redactor) -> Cow<'t, str> {
        match &self {
            Filter::Pattern(patterns) => {
                patterns
                    .iter()
                    .fold(Cow::Borrowed(input), |input, pattern| match pattern {
                        Pattern::Regex(regex) => {
                            regex.replace_all(&input, redactor).into_owned().into()
                        }
                        Pattern::String(pattern) => str_replace(&input, pattern, redactor).into(),
                    })
            }
            Filter::UsSocialSecurityNumber => {
                US_SOCIAL_SECURITY_NUMBER.replace_all(input, redactor)
            }
        }
    }
}

fn str_replace(haystack: &str, pattern: &str, redactor: &Redactor) -> String {
    let mut result = String::new();
    let mut last_end = 0;
    for (start, original) in haystack.match_indices(pattern) {
        result.push_str(&haystack[last_end..start]);
        redactor.replace_str(original, &mut result);
        last_end = start + original.len();
    }
    result.push_str(&haystack[last_end..]);
    result
}

#[allow(unpredictable_function_pointer_comparisons)]
/// The recipe for redacting the matched filters.
#[derive(Debug, Default, Clone, PartialEq, Eq)]
enum Redactor {
    #[default]
    Full,
    /// Replace with a fixed string.
    Text(String),
    /// Replace with a hash of the redacted content.
    Hash {
        encoder: Encoder,
        hasher: fn(Encoder, &[u8]) -> String,
    },
}

const REDACTED: &str = "[REDACTED]";

impl Redactor {
    fn replace_str(&self, original: &str, dst: &mut String) {
        match self {
            Redactor::Full => {
                dst.push_str(REDACTED);
            }
            Redactor::Text(s) => {
                dst.push_str(s);
            }
            Redactor::Hash { encoder, hasher } => {
                dst.push_str(&hasher(*encoder, original.as_bytes()));
            }
        }
    }

    fn from_object(obj: &ObjectMap) -> std::result::Result<Self, &'static str> {
        let r#type = match obj
            .get("type")
            .ok_or("redactor specified as objects must have type parameter")?
        {
            Value::Bytes(bytes) => Ok(bytes.clone()),
            _ => Err("type key in redactor must be a string"),
        }?;

        match r#type.as_ref() {
            b"full" => Ok(Redactor::Full),
            b"text" => match obj
                .get("replacement")
                .ok_or("text redactor must have `replacement` specified")?
            {
                Value::Bytes(bytes) => {
                    Ok(Redactor::Text(String::from_utf8_lossy(bytes).into_owned()))
                }
                _ => Err("`replacement` must be a string"),
            },
            b"sha2" => {
                let hasher = if let Some(variant) = obj.get("variant") {
                    match variant
                        .as_bytes()
                        .ok_or("`variant` must be a string")?
                        .as_ref()
                    {
                        b"SHA-224" => encoded_hash::<sha2::Sha224>,
                        b"SHA-256" => encoded_hash::<sha2::Sha256>,
                        b"SHA-384" => encoded_hash::<sha2::Sha384>,
                        b"SHA-512" => encoded_hash::<sha2::Sha512>,
                        b"SHA-512/224" => encoded_hash::<sha2::Sha512_224>,
                        b"SHA-512/256" => encoded_hash::<sha2::Sha512_256>,
                        _ => return Err("invalid sha2 variant"),
                    }
                } else {
                    encoded_hash::<sha2::Sha512_256>
                };
                let encoder = obj
                    .get("encoding")
                    .map(Encoder::try_from)
                    .transpose()?
                    .unwrap_or(Encoder::Base64);
                Ok(Redactor::Hash { hasher, encoder })
            }
            b"sha3" => {
                let hasher = if let Some(variant) = obj.get("variant") {
                    match variant
                        .as_bytes()
                        .ok_or("`variant` must be a string")?
                        .as_ref()
                    {
                        b"SHA3-224" => encoded_hash::<sha3::Sha3_224>,
                        b"SHA3-256" => encoded_hash::<sha3::Sha3_256>,
                        b"SHA3-384" => encoded_hash::<sha3::Sha3_384>,
                        b"SHA3-512" => encoded_hash::<sha3::Sha3_512>,
                        _ => return Err("invalid sha3 variant"),
                    }
                } else {
                    encoded_hash::<sha3::Sha3_512>
                };
                let encoder = obj
                    .get("encoding")
                    .map(Encoder::try_from)
                    .transpose()?
                    .unwrap_or(Encoder::Base64);
                Ok(Redactor::Hash { hasher, encoder })
            }
            _ => Err("unknown `type` for `redactor`"),
        }
    }
}

impl regex::Replacer for &Redactor {
    fn replace_append(&mut self, caps: &regex::Captures, dst: &mut String) {
        self.replace_str(&caps[0], dst);
    }

    fn no_expansion(&mut self) -> Option<Cow<'_, str>> {
        match self {
            Redactor::Full => Some(REDACTED.into()),
            Redactor::Text(s) => Some(s.into()),
            Redactor::Hash { .. } => None,
        }
    }
}

impl TryFrom<Value> for Redactor {
    type Error = &'static str;

    fn try_from(value: Value) -> std::result::Result<Self, Self::Error> {
        match value {
            Value::Object(object) => Redactor::from_object(&object),
            Value::Bytes(bytes) => match bytes.as_ref() {
                b"full" => Ok(Redactor::Full),
                b"sha2" => Ok(Redactor::Hash {
                    hasher: encoded_hash::<sha2::Sha512_256>,
                    encoder: Encoder::Base64,
                }),
                b"sha3" => Ok(Redactor::Hash {
                    hasher: encoded_hash::<sha3::Sha3_512>,
                    encoder: Encoder::Base64,
                }),
                _ => Err("unknown name of redactor"),
            },
            _ => Err("unknown literal for redactor, must be redactor name or object"),
        }
    }
}

#[derive(Debug, Copy, Clone, PartialEq, Eq)]
enum Encoder {
    Base64,
    Base16,
}

impl TryFrom<&Value> for Encoder {
    type Error = &'static str;

    fn try_from(value: &Value) -> std::result::Result<Self, Self::Error> {
        match value.as_bytes().ok_or("encoding must be string")?.as_ref() {
            b"base64" => Ok(Self::Base64),
            b"base16" | b"hex" => Ok(Self::Base16),
            _ => Err("unexpected encoding"),
        }
    }
}

impl Encoder {
    fn encode(self, data: &[u8]) -> String {
        use base64::Engine;
        match self {
            Encoder::Base64 => base64::engine::general_purpose::STANDARD.encode(data),
            Encoder::Base16 => base16::encode_lower(data),
        }
    }
}

/// Compute the hash of `data` using `T` as the digest, then encode it using `encoder`
/// to get a String.
fn encoded_hash<T: digest::Digest>(encoder: Encoder, data: &[u8]) -> String {
    encoder.encode(&T::digest(data))
}

#[cfg(test)]
mod tests {
    use super::*;
    use vrl::compiler::runtime::Runtime;
    use vrl::compiler::{TargetValue, TimeZone};
    use vrl::value::Secrets;

    fn resolve(source: &str) -> Result<Value, String> {
        let functions: Vec<Box<dyn Function>> = vec![Box::new(Redact)];
        let compiled = vrl::compiler::compile(source, &functions).map_err(|e| format!("{e:?}"))?;

        let mut target = TargetValue {
            value: Value::Object(ObjectMap::new()),
            metadata: Value::Object(ObjectMap::new()),
            secrets: Secrets::default(),
        };
        let mut runtime = Runtime::default();
        runtime
            .resolve(&mut target, &compiled.program, &TimeZone::default())
            .map_err(|e| format!("{e:?}"))
    }

    #[test]
    fn redacts_a_regex_match() {
        let result = resolve(r#"redact("hello 123456 world", [r'\d+'])"#).unwrap();
        assert_eq!(result.as_bytes().unwrap(), "hello [REDACTED] world");
    }

    #[test]
    fn redacts_us_social_security_numbers() {
        let result =
            resolve(r#"redact("ssn is 123-12-1234", ["us_social_security_number"])"#).unwrap();
        assert_eq!(result.as_bytes().unwrap(), "ssn is [REDACTED]");
    }

    #[test]
    fn redacts_with_custom_text() {
        let result =
            resolve(r#"redact("id 123456", [r'\d+'], {"type": "text", "replacement": "***"})"#)
                .unwrap();
        assert_eq!(result.as_bytes().unwrap(), "id ***");
    }

    #[test]
    fn redacts_with_sha2_hash() {
        let result = resolve(r#"redact("my id is 123456", [r'\d+'], "sha2")"#).unwrap();
        assert_eq!(
            result.as_bytes().unwrap(),
            "my id is GEtTedW1p6tC094dDKH+3B8P+xSnZz69AmpjaXRd63I="
        );
    }

    #[test]
    fn redacts_with_sha256_hex_encoding() {
        let result = resolve(
            r#"redact("my id is 123456", [r'\d+'], {"type": "sha2", "variant": "SHA-256", "encoding": "base16"})"#,
        )
        .unwrap();
        assert_eq!(
            result.as_bytes().unwrap(),
            "my id is 8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
        );
    }

    #[test]
    fn rejects_unknown_filter() {
        assert!(resolve(r#"redact("hello", ["not a filter"])"#).is_err());
    }
}
