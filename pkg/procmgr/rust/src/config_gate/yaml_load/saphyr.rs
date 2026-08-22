// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Event-driven YAML loader with Go yaml.v2 plain-scalar bool coercion.

use std::collections::HashMap;

use anyhow::{Context, Result, bail};
use saphyr_parser::{Event, Parser, ScalarStyle, Tag};
use serde_yaml::{Mapping, Sequence, Value};

pub(super) fn load(contents: &str) -> Result<Value> {
    let parser = Parser::new_from_str(contents);
    let mut builder = Builder::default();
    for result in parser {
        let (event, _) = result.context("parse YAML event")?;
        builder.push(event)?;
    }
    builder.finish()
}

fn scalar_to_value(text: &str, style: ScalarStyle, tag: Option<&Tag>) -> Value {
    if let Some(tag) = tag {
        return tagged_scalar_to_value(text, tag);
    }
    if style == ScalarStyle::Plain {
        plain_scalar_to_value(text)
    } else {
        Value::String(text.to_owned())
    }
}

/// Explicit YAML tags disable implicit plain-scalar resolution (Go yaml.v2 parity).
fn tagged_scalar_to_value(text: &str, tag: &Tag) -> Value {
    if !tag.is_yaml_core_schema() {
        return Value::String(text.to_owned());
    }
    match tag.suffix.as_str() {
        "str" => Value::String(text.to_owned()),
        "bool" => plain_scalar_to_value(text),
        "int" | "float" => {
            yaml_v2_plain_number(text).unwrap_or_else(|| Value::String(text.to_owned()))
        }
        "null" => Value::Null,
        _ => Value::String(text.to_owned()),
    }
}

/// Plain-scalar coercion aligned with Go yaml.v2 (YAML 1.1 bool/null spellings
/// and numeric scalars, including floats such as `1.0`).
fn plain_scalar_to_value(text: &str) -> Value {
    match text.to_ascii_lowercase().as_str() {
        "" | "~" | "null" => Value::Null,
        "true" | "yes" | "on" | "y" => Value::Bool(true),
        "false" | "no" | "off" | "n" => Value::Bool(false),
        _ => yaml_v2_plain_number(text).unwrap_or_else(|| Value::String(text.to_owned())),
    }
}

/// Integers first (decimal, then yaml.v2 `0x`/`0b`/`0o` prefixes), then YAML 1.1
/// special floats (`.inf`, `-.inf`, `.nan`), then finite decimal/scientific floats.
/// Matches yaml.v2 unmarshalling into `interface{}` so `cast.ToBoolE` can treat a
/// non-zero number as true.
fn yaml_v2_plain_number(text: &str) -> Option<Value> {
    let plain: String = text.chars().filter(|c| *c != '_').collect();
    if let Ok(n) = plain.parse::<i64>() {
        return Some(Value::Number(n.into()));
    }
    if let Ok(n) = plain.parse::<u64>() {
        return Some(Value::Number(n.into()));
    }
    if let Some(n) = parse_yaml_v2_prefixed_int(&plain) {
        return Some(n);
    }
    if let Some(n) = parse_yaml_v2_special_float(&plain) {
        return Some(Value::Number(n.into()));
    }
    if !looks_like_yaml_11_float(&plain) {
        return None;
    }
    plain
        .parse::<f64>()
        .ok()
        .filter(|n| n.is_finite())
        .map(|n| Value::Number(n.into()))
}

/// YAML 1.1 special floats resolved by go.yaml.in/yaml/v2 (must include a leading `.`).
fn parse_yaml_v2_special_float(text: &str) -> Option<f64> {
    if let Some(rest) = text.strip_prefix('+') {
        return parse_yaml_v2_special_float_unsigned(rest);
    }
    if let Some(rest) = text.strip_prefix('-') {
        return parse_yaml_v2_special_float_unsigned(rest)
            .and_then(|n| if n.is_infinite() { Some(-n) } else { None });
    }
    parse_yaml_v2_special_float_unsigned(text)
}

fn parse_yaml_v2_special_float_unsigned(text: &str) -> Option<f64> {
    let suffix = text.strip_prefix('.')?;
    if suffix.eq_ignore_ascii_case("inf") {
        Some(f64::INFINITY)
    } else if suffix.eq_ignore_ascii_case("nan") {
        Some(f64::NAN)
    } else {
        None
    }
}

/// `strconv.ParseInt`/`ParseUint` with base 0: `0x` hex, `0b` binary, `0o` octal.
/// Leading-zero decimals stay decimal (`010` is 10) so `08` still parses as 8
/// instead of failing octal and falling through to a string.
fn parse_yaml_v2_prefixed_int(plain: &str) -> Option<Value> {
    let (negative, rest) = match plain.as_bytes().first() {
        Some(b'+') => (false, &plain[1..]),
        Some(b'-') => (true, &plain[1..]),
        _ => (false, plain),
    };
    let bytes = rest.as_bytes();
    if bytes.len() < 3 || bytes[0] != b'0' {
        return None;
    }
    let (base, digits) = match bytes[1] {
        b'x' | b'X' => (16, &rest[2..]),
        b'b' | b'B' => (2, &rest[2..]),
        b'o' | b'O' => (8, &rest[2..]),
        _ => return None,
    };
    if digits.is_empty() {
        return None;
    }
    if negative {
        let n = i64::from_str_radix(digits, base).ok()?;
        return Some(Value::Number(n.checked_neg()?.into()));
    }
    if let Ok(n) = i64::from_str_radix(digits, base) {
        return Some(Value::Number(n.into()));
    }
    let n = u64::from_str_radix(digits, base).ok()?;
    Some(Value::Number(n.into()))
}

/// Digit-based decimal or scientific form (`1.0`, `1e0`). Plain `inf` without a
/// leading dot stays a string in yaml.v2.
fn looks_like_yaml_11_float(text: &str) -> bool {
    let mut rest = text.as_bytes();
    if rest.first().is_some_and(|c| *c == b'+' || *c == b'-') {
        rest = &rest[1..];
    }
    if rest.is_empty() {
        return false;
    }
    let has_digit = rest.iter().any(u8::is_ascii_digit);
    let has_dot_or_exp = rest.iter().any(|c| *c == b'.' || *c == b'e' || *c == b'E');
    let all_float_chars = rest.iter().all(|c| {
        c.is_ascii_digit() || *c == b'.' || *c == b'e' || *c == b'E' || *c == b'+' || *c == b'-'
    });
    has_digit && has_dot_or_exp && all_float_chars
}

fn mapping_from_pairs(pairs: HashMap<String, Value>) -> Value {
    let mut map = Mapping::new();
    for (key, value) in pairs {
        map.insert(Value::String(key), value);
    }
    Value::Mapping(map)
}

fn scalar_as_key(value: Value) -> String {
    match value {
        Value::String(text) => text,
        Value::Bool(enabled) => enabled.to_string(),
        Value::Number(number) => number.to_string(),
        Value::Null => "null".to_owned(),
        _ => String::new(),
    }
}

#[derive(Default)]
struct Builder {
    stack: Vec<Frame>,
    anchors: HashMap<usize, Value>,
    root: Option<Value>,
    /// After the first document is parsed, ignore later `---` documents (Go yaml.v2 parity).
    ignore_remaining_documents: bool,
}

enum Frame {
    Mapping {
        pairs: HashMap<String, Value>,
        pending_key: Option<String>,
        anchor: usize,
    },
    Sequence {
        items: Vec<Value>,
        anchor: usize,
    },
}

impl Builder {
    fn push(&mut self, event: Event<'_>) -> Result<()> {
        if self.ignore_remaining_documents {
            return Ok(());
        }
        match event {
            Event::Nothing | Event::StreamStart | Event::StreamEnd | Event::DocumentEnd => {}
            Event::DocumentStart(_) => {
                if self.root.is_some() {
                    self.ignore_remaining_documents = true;
                    return Ok(());
                }
                self.stack.clear();
                self.anchors.clear();
            }
            Event::MappingStart(anchor, _) => self.stack.push(Frame::Mapping {
                pairs: HashMap::new(),
                pending_key: None,
                anchor,
            }),
            Event::MappingEnd => self.finish_mapping()?,
            Event::SequenceStart(anchor, _) => self.stack.push(Frame::Sequence {
                items: Vec::new(),
                anchor,
            }),
            Event::SequenceEnd => self.finish_sequence()?,
            Event::Scalar(text, style, anchor, tag) => {
                let value = scalar_to_value(&text, style, tag.as_deref());
                self.store_anchor(anchor, &value);
                self.attach(value);
            }
            Event::Alias(anchor) => {
                let value = self
                    .anchors
                    .get(&anchor)
                    .with_context(|| format!("unknown YAML alias anchor {anchor}"))?
                    .clone();
                self.attach(value);
            }
        }
        Ok(())
    }

    fn finish_mapping(&mut self) -> Result<()> {
        let frame = self.stack.pop().context("unexpected YAML mapping end")?;
        let (pairs, anchor) = match frame {
            Frame::Mapping {
                pairs,
                pending_key: None,
                anchor,
            } => (pairs, anchor),
            Frame::Mapping {
                pending_key: Some(_),
                ..
            } => {
                bail!("YAML mapping ended before value for key");
            }
            _ => bail!("YAML mapping end without matching start"),
        };
        self.attach_anchored(mapping_from_pairs(pairs), anchor);
        Ok(())
    }

    fn finish_sequence(&mut self) -> Result<()> {
        let frame = self.stack.pop().context("unexpected YAML sequence end")?;
        let (items, anchor) = match frame {
            Frame::Sequence { items, anchor } => (items, anchor),
            _ => bail!("YAML sequence end without matching start"),
        };
        self.attach_anchored(Value::Sequence(Sequence::from(items)), anchor);
        Ok(())
    }

    fn store_anchor(&mut self, anchor: usize, value: &Value) {
        if anchor != 0 {
            self.anchors.insert(anchor, value.clone());
        }
    }

    fn attach_anchored(&mut self, value: Value, anchor: usize) {
        self.store_anchor(anchor, &value);
        self.attach(value);
    }

    fn attach(&mut self, value: Value) {
        match self.stack.last_mut() {
            None => self.root = Some(value),
            Some(Frame::Mapping {
                pairs, pending_key, ..
            }) => {
                if pending_key.is_none() {
                    *pending_key = Some(scalar_as_key(value));
                } else {
                    pairs.insert(pending_key.take().expect("mapping key"), value);
                }
            }
            Some(Frame::Sequence { items, .. }) => items.push(value),
        }
    }

    fn finish(self) -> Result<Value> {
        if !self.stack.is_empty() {
            bail!("incomplete YAML document");
        }
        self.root.context("empty YAML document")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn multi_document_stream_uses_first_document() {
        let yaml = "process_config:\n  process_collection:\n    enabled: true\n---\nprocess_config:\n  process_collection:\n    enabled: false\n";
        let root = load(yaml).unwrap();
        assert_eq!(
            root.get("process_config")
                .and_then(|pc| pc.get("process_collection"))
                .and_then(|col| col.get("enabled"))
                .and_then(Value::as_bool),
            Some(true)
        );
    }

    #[test]
    fn plain_yaml_11_bool_coerced_at_load() {
        let root = load("enabled: yes\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::Bool(true)));
    }

    #[test]
    fn tagged_str_yaml_11_bool_stays_string() {
        let root = load("enabled: !!str yes\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("yes".into())));
    }

    #[test]
    fn tagged_bool_yaml_11_bool_coerced_at_load() {
        let root = load("enabled: !!bool yes\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::Bool(true)));
    }

    #[test]
    fn quoted_yaml_11_bool_stays_string() {
        let root = load("enabled: \"yes\"\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("yes".into())));
    }

    #[test]
    fn single_quoted_yaml_11_bool_stays_string() {
        let root = load("enabled: 'on'\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("on".into())));
    }

    #[test]
    fn plain_float_is_number() {
        let root = load("enabled: 1.0\n").unwrap();
        assert_eq!(root.get("enabled").and_then(Value::as_f64), Some(1.0));
    }

    #[test]
    fn quoted_float_stays_string() {
        let root = load("enabled: \"1.0\"\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("1.0".into())));
    }

    #[test]
    fn plain_dot_inf_is_number() {
        let root = load("enabled: .inf\n").unwrap();
        assert!(
            root.get("enabled")
                .and_then(Value::as_f64)
                .unwrap()
                .is_infinite()
        );
    }

    #[test]
    fn plain_dot_nan_is_number() {
        let root = load("enabled: .nan\n").unwrap();
        assert!(
            root.get("enabled")
                .and_then(Value::as_f64)
                .unwrap()
                .is_nan()
        );
    }

    #[test]
    fn negative_dot_nan_stays_string() {
        let root = load("enabled: -.nan\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("-.nan".into())));
    }

    #[test]
    fn rust_inf_spelling_stays_string() {
        let root = load("enabled: inf\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("inf".into())));
    }

    #[test]
    fn plain_u64_max_decimal_is_number() {
        let root = load("enabled: 18446744073709551615\n").unwrap();
        assert_eq!(root.get("enabled").and_then(Value::as_u64), Some(u64::MAX));
    }

    #[test]
    fn plain_u64_max_hex_is_number() {
        let root = load("enabled: 0xffffffffffffffff\n").unwrap();
        assert_eq!(root.get("enabled").and_then(Value::as_u64), Some(u64::MAX));
    }

    #[test]
    fn plain_prefixed_ints_are_numbers() {
        assert_eq!(
            load("enabled: 0x1\n")
                .unwrap()
                .get("enabled")
                .and_then(Value::as_i64),
            Some(1)
        );
        assert_eq!(
            load("enabled: 0b1\n")
                .unwrap()
                .get("enabled")
                .and_then(Value::as_i64),
            Some(1)
        );
        assert_eq!(
            load("enabled: 0o1\n")
                .unwrap()
                .get("enabled")
                .and_then(Value::as_i64),
            Some(1)
        );
        assert_eq!(
            load("enabled: 0x0\n")
                .unwrap()
                .get("enabled")
                .and_then(Value::as_i64),
            Some(0)
        );
        assert_eq!(
            load("enabled: -0x1\n")
                .unwrap()
                .get("enabled")
                .and_then(Value::as_i64),
            Some(-1)
        );
    }

    #[test]
    fn quoted_hex_int_stays_string() {
        let root = load("enabled: \"0x1\"\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("0x1".into())));
    }
}
