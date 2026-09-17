// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Agent YAML config file semantics, without the Go config loader.
//!
//! Reads `datadog.yaml` / `system-probe.yaml` the way the Agent does, so procmgr can
//! answer config questions about a file it does not own:
//!
//! * [`load`] parses with [`saphyr_parser`] so plain scalars resolve like Go
//!   `gopkg.in/yaml.v2`: YAML 1.1 bool spellings (`yes`/`on`/…) become bools, quoted and
//!   `!!str`-tagged scalars stay strings, and duplicate mapping keys last-win.
//! * [`lookup_dotted_key`] resolves `a.b.c` against both nested and flattened mappings,
//!   case-insensitively, mirroring the key expansion in
//!   `pkg/config/nodetreemodel/read_config_file.go`.
//! * [`value_as_bool`] / [`scalar_as_string`] mirror `GetBool` / `GetString` coercion
//!   (`cast.ToBoolE`, `strconv.ParseBool`).
//!
//! Nothing here knows about environment variables, fleet policy, or defaults; that
//! resolution lives in [`crate::config_gate`].

use anyhow::{Context, Result, bail};
use base64::prelude::{BASE64_STANDARD, Engine as _};
use saphyr_parser::{Event, Parser, ScalarStyle, Tag};
use serde_yaml::{Mapping, Sequence, Value};
use std::collections::HashMap;

/// Parses the first document of an Agent YAML file.
///
/// An empty or comment-only file is a valid config that sets nothing, so it yields
/// [`Value::Null`] rather than an error.
pub fn load(contents: &str) -> Result<Value> {
    let mut builder = Builder::default();
    for result in Parser::new_from_str(contents) {
        let (event, _) = result.context("parse YAML event")?;
        builder.push(event)?;
    }
    let mut root = builder.finish()?;
    root.apply_merge().context("apply YAML merge keys")?;
    Ok(root)
}

/// Resolves a dotted config key, mirroring the Agent's flattened-key expansion.
///
/// `loadYamlInto` in `read_config_file.go` expands every mapping key containing a `.`
/// into nested maps, so `a.b:` with a child `c` and a plain `a: {b: {c: …}}` are the
/// same setting to the Agent. serde_yaml keeps the literal key, so each mapping is
/// searched for the whole remaining key first, then for any dotted prefix of it.
pub fn lookup_dotted_key<'a>(root: &'a Value, key: &str) -> Option<&'a Value> {
    let mapping = root.as_mapping()?;

    if let Some(value) = lookup_case_insensitive(mapping, key) {
        return value_is_set(value).then_some(value);
    }

    key.match_indices('.').find_map(|(dot, _)| {
        let child = lookup_case_insensitive(mapping, &key[..dot])?;
        lookup_dotted_key(child, &key[dot + 1..])
    })
}

/// Whether a node counts as an explicit config value.
///
/// Mirrors `read_config_file.go`: a known leaf with no value (`setting_name:`) is
/// ignored and does not mark the key as configured.
fn value_is_set(value: &Value) -> bool {
    !matches!(value, Value::Null)
}

fn lookup_case_insensitive<'a>(mapping: &'a Mapping, key: &str) -> Option<&'a Value> {
    if let Some(value) = mapping.get(key) {
        return Some(value);
    }
    mapping.iter().find_map(|(k, v)| {
        k.as_str()
            .filter(|segment| segment.eq_ignore_ascii_case(key))
            .map(|_| v)
    })
}

/// Mirrors Go `GetBool` coercion. Returns `None` for collections, which `cast.ToBoolE`
/// also refuses.
pub fn value_as_bool(value: &Value) -> Option<bool> {
    match value {
        // Plain YAML 1.1 bools (`yes`/`on`/…) are coerced to bool at load time.
        Value::Bool(enabled) => Some(*enabled),
        // `cast.ToBoolE`: any non-zero number is true, including yaml.v2 floats such as `1.0`.
        Value::Number(number) => Some(number_as_bool(number)),
        // Quoted scalars and env vars: `strconv.ParseBool` only.
        Value::String(text) => Some(parse_bool_string(text).unwrap_or(false)),
        _ => None,
    }
}

fn number_as_bool(number: &serde_yaml::Number) -> bool {
    if let Some(n) = number.as_i64() {
        n != 0
    } else if let Some(n) = number.as_u64() {
        n != 0
    } else if let Some(n) = number.as_f64() {
        n != 0.0
    } else {
        false
    }
}

/// Mirrors Go `strconv.ParseBool`, used for env vars and quoted YAML scalars.
///
/// The spellings are an exact list rather than a case-insensitive comparison: Go rejects
/// `tRuE`, and `GetBool` then reads it as false, so accepting it here would start a
/// process the Agent considers disabled.
pub fn parse_bool_string(text: &str) -> Option<bool> {
    match text {
        "1" | "t" | "T" | "true" | "TRUE" | "True" => Some(true),
        "0" | "f" | "F" | "false" | "FALSE" | "False" => Some(false),
        _ => None,
    }
}

/// Mirrors Go `GetString` for scalar nodes: strings pass through, bools and numbers
/// stringify, collections yield `None`.
pub fn scalar_as_string(value: &Value) -> Option<String> {
    match value {
        Value::String(text) => Some(text.clone()),
        Value::Bool(enabled) => Some(enabled.to_string()),
        Value::Number(number) => Some(number_as_string(number)),
        _ => None,
    }
}

fn number_as_string(number: &serde_yaml::Number) -> String {
    if let Some(value) = number.as_i64() {
        return value.to_string();
    }
    if let Some(value) = number.as_u64() {
        return value.to_string();
    }
    match number.as_f64() {
        Some(value) if value.fract() == 0.0 && value.is_finite() => format!("{}", value as i64),
        Some(value) => value.to_string(),
        None => String::new(),
    }
}

fn scalar_to_value(text: &str, style: ScalarStyle, tag: Option<&Tag>) -> Result<Value> {
    if let Some(tag) = tag {
        return tagged_scalar_to_value(text, tag);
    }
    if style == ScalarStyle::Plain {
        Ok(plain_scalar_to_value(text))
    } else {
        Ok(Value::String(text.to_owned()))
    }
}

/// Applies an explicit YAML tag the way yaml.v2's `resolve` does.
///
/// A tag does not reinterpret the scalar, it constrains it: yaml.v2 resolves the text as
/// if it were plain and then calls `failf` when the result does not match the tag, so
/// `!!bool 1` and `!!int 1.0` abort `Unmarshal` instead of being coerced. Coercing them
/// here would hand a gate a value the Agent never sees, because the Agent fails to load
/// the file at all, so the mismatch is surfaced as a [`load`] error.
fn tagged_scalar_to_value(text: &str, tag: &Tag) -> Result<Value> {
    if !tag.is_yaml_core_schema() {
        // `resolvableTag` is false for a non-`tag:yaml.org,2002:` handle, so the scalar
        // passes through untouched.
        return Ok(Value::String(text.to_owned()));
    }
    match tag.suffix.as_str() {
        // Any scalar is accepted as a `!!str`.
        "str" => Ok(Value::String(text.to_owned())),
        "binary" => decode_binary(text),
        "bool" => resolved_as(text, "bool", |value| matches!(value, Value::Bool(_))),
        "null" => resolved_as(text, "null", |value| matches!(value, Value::Null)),
        "int" => resolved_as(
            text,
            "int",
            |value| matches!(value, Value::Number(number) if !number.is_f64()),
        ),
        "float" => tagged_float(text),
        // yaml.v2 resolves `!!timestamp` to a `time.Time`, which no config gate reads.
        // Keeping the scalar verbatim is closer than failing a file the Agent accepts.
        //
        // Every other suffix is unresolvable, so the scalar passes through.
        _ => Ok(Value::String(text.to_owned())),
    }
}

fn resolved_as(text: &str, suffix: &str, accepts: fn(&Value) -> bool) -> Result<Value> {
    let resolved = plain_scalar_to_value(text);
    if !accepts(&resolved) {
        bail!("cannot decode `{text}` as !!{suffix}");
    }
    Ok(resolved)
}

/// `!!float` is the one tag yaml.v2 widens rather than rejects: an integer scalar becomes
/// a float, anything non-numeric still fails.
fn tagged_float(text: &str) -> Result<Value> {
    let Value::Number(number) = plain_scalar_to_value(text) else {
        bail!("cannot decode `{text}` as !!float");
    };
    if number.is_f64() {
        return Ok(Value::Number(number));
    }
    let widened = number
        .as_f64()
        .with_context(|| format!("cannot decode `{text}` as !!float"))?;
    Ok(Value::Number(widened.into()))
}

/// Decodes a `!!binary` payload, which yaml.v2 hands over as the decoded bytes rather
/// than the base64 text, so a gate compares against the same string the Agent sees.
fn decode_binary(text: &str) -> Result<Value> {
    // Go's `base64.StdEncoding` ignores line breaks, which the YAML spec allows inside a
    // binary payload, but no other whitespace.
    let payload: String = text.chars().filter(|c| *c != '\r' && *c != '\n').collect();
    let bytes = BASE64_STANDARD
        .decode(payload)
        .context("!!binary value contains invalid base64 data")?;
    // Go keeps the raw bytes in a string. Rust cannot, and no config gate reads a
    // non-UTF-8 setting, so replace rather than fail a file the Agent loads.
    Ok(Value::String(String::from_utf8_lossy(&bytes).into_owned()))
}

/// Plain-scalar coercion aligned with Go yaml.v2: YAML 1.1 bool and null spellings,
/// then numeric scalars (including floats such as `1.0`).
///
/// The spellings come from `resolveMap` in yaml.v2's `resolve.go`, which is an exact
/// lookup table covering only the lowercase, capitalized, and uppercase forms. Matching
/// case-insensitively instead would turn `tRuE` into a bool here while the Agent keeps
/// it a string that `GetBool` reads as false.
fn plain_scalar_to_value(text: &str) -> Value {
    match text {
        "" | "~" | "null" | "Null" | "NULL" => Value::Null,
        "y" | "Y" | "yes" | "Yes" | "YES" | "true" | "True" | "TRUE" | "on" | "On" | "ON" => {
            Value::Bool(true)
        }
        "n" | "N" | "no" | "No" | "NO" | "false" | "False" | "FALSE" | "off" | "Off" | "OFF" => {
            Value::Bool(false)
        }
        _ => plain_number(text).unwrap_or_else(|| Value::String(text.to_owned())),
    }
}

/// Integers first (decimal, then yaml.v2 `0x`/`0b`/`0o` prefixes), then YAML 1.1
/// special floats (`.inf`, `-.inf`, `.nan`), then finite decimal/scientific floats.
///
/// Matches yaml.v2 unmarshalling into `interface{}`, so `cast.ToBoolE` sees a number
/// (any non-zero is true) wherever Go would.
fn plain_number(text: &str) -> Option<Value> {
    let plain: String = text.chars().filter(|c| *c != '_').collect();
    if let Ok(n) = plain.parse::<i64>() {
        return Some(Value::Number(n.into()));
    }
    if let Ok(n) = plain.parse::<u64>() {
        return Some(Value::Number(n.into()));
    }
    if let Some(n) = prefixed_int(&plain) {
        return Some(n);
    }
    if let Some(n) = special_float(&plain) {
        return Some(Value::Number(n.into()));
    }
    if !looks_like_float(&plain) {
        return None;
    }
    plain
        .parse::<f64>()
        .ok()
        .filter(|n| n.is_finite())
        .map(|n| Value::Number(n.into()))
}

/// YAML 1.1 special floats resolved by go.yaml.in/yaml/v2.
///
/// Another exact `resolveMap` slice: only these spellings are floats, so `.iNf` and
/// `+.nan` (which yaml.v2 has no entry for) stay strings.
fn special_float(text: &str) -> Option<f64> {
    match text {
        ".nan" | ".NaN" | ".NAN" => Some(f64::NAN),
        ".inf" | ".Inf" | ".INF" | "+.inf" | "+.Inf" | "+.INF" => Some(f64::INFINITY),
        "-.inf" | "-.Inf" | "-.INF" => Some(f64::NEG_INFINITY),
        _ => None,
    }
}

/// `strconv.ParseInt`/`ParseUint` with base 0: `0x` hex, `0b` binary, `0o` octal.
/// Leading-zero decimals stay decimal (`010` is 10) so `08` still parses as 8
/// instead of failing octal and falling through to a string.
fn prefixed_int(plain: &str) -> Option<Value> {
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
        if let Ok(n) = i64::from_str_radix(digits, base) {
            return Some(Value::Number(n.checked_neg()?.into()));
        }
        // Magnitude 2^63 (e.g. 0x8000000000000000) exceeds i64::MAX but negates to i64::MIN.
        let magnitude = u64::from_str_radix(digits, base).ok()?;
        if magnitude == i64::MIN.unsigned_abs() {
            return Some(Value::Number(i64::MIN.into()));
        }
        return None;
    }
    if let Ok(n) = i64::from_str_radix(digits, base) {
        return Some(Value::Number(n.into()));
    }
    let n = u64::from_str_radix(digits, base).ok()?;
    Some(Value::Number(n.into()))
}

/// Digit-based decimal or scientific form (`1.0`, `1e0`). Plain `inf` without a
/// leading dot stays a string in yaml.v2.
fn looks_like_float(text: &str) -> bool {
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

/// Event-driven document builder. Mapping keys go through a [`HashMap`] so duplicate
/// keys last-win, matching Go `yaml.Unmarshal`.
#[derive(Default)]
struct Builder {
    stack: Vec<Frame>,
    anchors: HashMap<usize, Value>,
    root: Option<Value>,
    /// After the first document is parsed, ignore later `---` documents (yaml.v2 parity).
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
                let value = scalar_to_value(&text, style, tag.as_deref())?;
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
            } => bail!("YAML mapping ended before value for key"),
            Frame::Sequence { .. } => bail!("YAML mapping end without matching start"),
        };
        self.attach_anchored(mapping_from_pairs(pairs), anchor);
        Ok(())
    }

    fn finish_sequence(&mut self) -> Result<()> {
        let frame = self.stack.pop().context("unexpected YAML sequence end")?;
        let (items, anchor) = match frame {
            Frame::Sequence { items, anchor } => (items, anchor),
            Frame::Mapping { .. } => bail!("YAML sequence end without matching start"),
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
        Ok(self.root.unwrap_or(Value::Null))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn dotted(yaml: &str, key: &str) -> Option<Value> {
        let root = load(yaml).unwrap();
        lookup_dotted_key(&root, key).cloned()
    }

    fn scalar(yaml: &str) -> Value {
        dotted(yaml, "enabled").unwrap_or(Value::Null)
    }

    #[test]
    fn comment_only_file_is_an_empty_config() {
        // The most common shape of datadog.yaml for these gates: no keys set at all.
        assert_eq!(load("# api_key: placeholder\n").unwrap(), Value::Null);
        assert_eq!(load("").unwrap(), Value::Null);
        assert_eq!(dotted("# nothing here\n", "process_config.enabled"), None);
    }

    #[test]
    fn malformed_yaml_is_an_error() {
        assert!(load("{\n").is_err());
    }

    #[test]
    fn multi_document_stream_uses_first_document() {
        assert_eq!(
            dotted(
                "process_config:\n  enabled: true\n---\nprocess_config:\n  enabled: false\n",
                "process_config.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    #[test]
    fn duplicate_keys_last_win() {
        assert_eq!(
            dotted(
                "process_config:\n  enabled: false\nprocess_config:\n  enabled: true\n",
                "process_config.enabled"
            ),
            Some(Value::Bool(true))
        );
        assert_eq!(
            dotted(
                "process_config:\n  process_collection:\n    enabled: false\n  process_collection:\n    enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    #[test]
    fn merge_keys_are_expanded() {
        let yaml = r#"
disabled: &disabled
  process_collection:
    enabled: false

process_config:
  <<: *disabled
"#;
        assert_eq!(
            dotted(yaml, "process_config.process_collection.enabled"),
            Some(Value::Bool(false))
        );
    }

    #[test]
    fn dotted_lookup_is_case_insensitive() {
        assert_eq!(
            dotted(
                "process_config:\n  Process_Collection:\n    enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    #[test]
    fn dotted_lookup_supports_flattened_and_partially_flattened_keys() {
        assert_eq!(
            dotted(
                "process_config.process_collection.enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
        assert_eq!(
            dotted(
                "process_config:\n  process_collection.enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
        assert_eq!(
            dotted(
                "process_config.process_collection:\n  enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    /// A dotted prefix that leads nowhere must not stop the search: the Agent flattens
    /// every mapping key, so the value is still reachable through another split.
    #[test]
    fn dotted_lookup_keeps_searching_past_a_dead_end_prefix() {
        assert_eq!(
            dotted(
                "process_config: {}\nprocess_config.process_collection:\n  enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    #[test]
    fn valueless_leaf_counts_as_absent() {
        assert_eq!(
            dotted(
                "network_config:\n  enabled:\nsystem_probe_config:\n  enabled: true\n",
                "network_config.enabled"
            ),
            None
        );
        assert_eq!(
            dotted("network_config.enabled:\n", "network_config.enabled"),
            None
        );
    }

    #[test]
    fn plain_yaml_11_bools_are_bools() {
        assert_eq!(scalar("enabled: yes\n"), Value::Bool(true));
        assert_eq!(scalar("enabled: on\n"), Value::Bool(true));
        assert_eq!(scalar("enabled: !!bool yes\n"), Value::Bool(true));
    }

    /// yaml.v2 resolves plain scalars through an exact table, so only the lowercase,
    /// capitalized, and uppercase spellings of each keyword are bools or nulls.
    #[test]
    fn plain_bool_and_null_spellings_are_exact() {
        for text in [
            "y", "Y", "yes", "Yes", "YES", "true", "True", "TRUE", "on", "On", "ON",
        ] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::Bool(true),
                "{text}"
            );
        }
        for text in [
            "n", "N", "no", "No", "NO", "false", "False", "FALSE", "off", "Off", "OFF",
        ] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::Bool(false),
                "{text}"
            );
        }
        for text in ["~", "null", "Null", "NULL"] {
            assert_eq!(
                dotted(&format!("enabled: {text}\n"), "enabled"),
                None,
                "{text}"
            );
        }
    }

    #[test]
    fn mixed_case_keyword_spellings_stay_strings() {
        for text in ["tRuE", "yEs", "oN", "fAlSe", "nO", "oFf", "nUlL", "yES"] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::String(text.into()),
                "{text}"
            );
        }
    }

    #[test]
    fn special_float_spellings_are_exact() {
        for text in [".inf", ".Inf", ".INF", "+.inf", "+.Inf", "+.INF"] {
            let value = scalar(&format!("enabled: {text}\n"));
            assert_eq!(value.as_f64(), Some(f64::INFINITY), "{text}");
        }
        for text in ["-.inf", "-.Inf", "-.INF"] {
            let value = scalar(&format!("enabled: {text}\n"));
            assert_eq!(value.as_f64(), Some(f64::NEG_INFINITY), "{text}");
        }
        for text in [".nan", ".NaN", ".NAN"] {
            assert!(
                scalar(&format!("enabled: {text}\n"))
                    .as_f64()
                    .unwrap()
                    .is_nan(),
                "{text}"
            );
        }
        // yaml.v2 has no `+.nan` entry and no case-insensitive match.
        for text in ["+.nan", "-.nan", ".iNf", ".nAn"] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::String(text.into()),
                "{text}"
            );
        }
    }

    /// yaml.v2 resolves a tagged scalar as if it were plain and then rejects it when the
    /// result does not match the tag, which aborts the whole `Unmarshal`.
    #[test]
    fn tagged_scalars_must_match_their_tag() {
        for yaml in [
            "enabled: !!bool 1\n",
            "enabled: !!bool maybe\n",
            "enabled: !!int 1.0\n",
            "enabled: !!int yes\n",
            "enabled: !!float nope\n",
            "enabled: !!null nope\n",
        ] {
            assert!(load(yaml).is_err(), "{yaml:?}");
        }
    }

    #[test]
    fn tagged_scalars_resolve_like_yaml_v2() {
        assert_eq!(scalar("enabled: !!bool on\n"), Value::Bool(true));
        assert_eq!(scalar("enabled: !!int 0x10\n").as_i64(), Some(16));
        assert_eq!(scalar("enabled: !!float 1.5\n").as_f64(), Some(1.5));
        // `!!float` is widened from an integer instead of being rejected.
        assert_eq!(scalar("enabled: !!float 1\n").as_f64(), Some(1.0));
        assert!(load("enabled: !!null ~\n").is_ok());
        // A non-core handle is unresolvable, so the scalar passes through verbatim.
        assert_eq!(
            scalar("enabled: !custom yes\n"),
            Value::String("yes".into())
        );
    }

    #[test]
    fn binary_scalars_are_base64_decoded() {
        assert_eq!(
            scalar("enabled: !!binary dHJ1ZQ==\n"),
            Value::String("true".into())
        );
        assert!(load("enabled: !!binary \"not base64\"\n").is_err());
    }

    #[test]
    fn quoted_and_tagged_str_scalars_stay_strings() {
        assert_eq!(scalar("enabled: \"yes\"\n"), Value::String("yes".into()));
        assert_eq!(scalar("enabled: 'on'\n"), Value::String("on".into()));
        assert_eq!(scalar("enabled: !!str yes\n"), Value::String("yes".into()));
        assert_eq!(scalar("enabled: \"1.0\"\n"), Value::String("1.0".into()));
        assert_eq!(scalar("enabled: \"0x1\"\n"), Value::String("0x1".into()));
    }

    #[test]
    fn plain_numeric_scalars_are_numbers() {
        assert_eq!(scalar("enabled: 1.0\n").as_f64(), Some(1.0));
        assert_eq!(
            scalar("enabled: 18446744073709551615\n").as_u64(),
            Some(u64::MAX)
        );
        assert_eq!(scalar("enabled: 0x1\n").as_i64(), Some(1));
        assert_eq!(scalar("enabled: 0b1\n").as_i64(), Some(1));
        assert_eq!(scalar("enabled: 0o1\n").as_i64(), Some(1));
        assert_eq!(scalar("enabled: 0x0\n").as_i64(), Some(0));
        assert_eq!(scalar("enabled: -0x1\n").as_i64(), Some(-1));
        assert_eq!(
            scalar("enabled: 0xffffffffffffffff\n").as_u64(),
            Some(u64::MAX)
        );
        assert_eq!(
            scalar("enabled: -0x8000000000000000\n").as_i64(),
            Some(i64::MIN)
        );
        assert_eq!(
            scalar(
                "enabled: -0b1000000000000000000000000000000000000000000000000000000000000000\n"
            )
            .as_i64(),
            Some(i64::MIN)
        );
        assert!(scalar("enabled: .inf\n").as_f64().unwrap().is_infinite());
        assert!(scalar("enabled: .nan\n").as_f64().unwrap().is_nan());
    }

    #[test]
    fn non_yaml_v2_number_spellings_stay_strings() {
        assert_eq!(scalar("enabled: inf\n"), Value::String("inf".into()));
        assert_eq!(scalar("enabled: -.nan\n"), Value::String("-.nan".into()));
    }

    #[test]
    fn value_as_bool_mirrors_go_get_bool() {
        for (value, expected) in [
            (Value::Bool(true), Some(true)),
            (Value::String("true".into()), Some(true)),
            (Value::String("1".into()), Some(true)),
            (Value::String("disabled".into()), Some(false)),
            // Quoted scalars go through ParseBool only, so YAML 1.1 spellings are false.
            (Value::String("yes".into()), Some(false)),
            // ParseBool accepts "true"/"True"/"TRUE" but no other casing.
            (Value::String("True".into()), Some(true)),
            (Value::String("tRuE".into()), Some(false)),
            (Value::Number(1.into()), Some(true)),
            (Value::Number(0.into()), Some(false)),
            (Value::Number(1.0.into()), Some(true)),
            (Value::Number(0.0.into()), Some(false)),
            (Value::Sequence(Sequence::new()), None),
        ] {
            assert_eq!(value_as_bool(&value), expected, "value={value:?}");
        }
    }

    #[test]
    fn parse_bool_string_matches_strconv_parse_bool() {
        for (input, expected) in [
            ("1", Some(true)),
            ("t", Some(true)),
            ("T", Some(true)),
            ("true", Some(true)),
            ("TRUE", Some(true)),
            ("True", Some(true)),
            ("0", Some(false)),
            ("f", Some(false)),
            ("F", Some(false)),
            ("false", Some(false)),
            ("FALSE", Some(false)),
            ("False", Some(false)),
            ("yes", None),
            ("on", None),
            ("disabled", None),
            (" true ", None),
            (" false ", None),
            (" 1 ", None),
            // ParseBool matches an exact list, so any other casing is a syntax error.
            ("tRuE", None),
            ("trUE", None),
            ("FaLsE", None),
            ("fALSE", None),
        ] {
            assert_eq!(parse_bool_string(input), expected, "input={input:?}");
        }
    }

    #[test]
    fn scalar_as_string_mirrors_go_get_string() {
        for (value, expected) in [
            (Value::String("disabled".into()), Some("disabled")),
            (Value::Bool(true), Some("true")),
            (Value::Number(1.into()), Some("1")),
            (Value::Number((-1).into()), Some("-1")),
            (Value::Number(1.0.into()), Some("1")),
            (Value::Sequence(Sequence::new()), None),
        ] {
            assert_eq!(
                scalar_as_string(&value).as_deref(),
                expected,
                "value={value:?}"
            );
        }
    }
}
