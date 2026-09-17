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
/// An empty, comment-only or explicitly null file is a valid config that sets nothing,
/// so it yields [`Value::Null`] rather than an error. Any other non-mapping root is an
/// error, because `readConfigurationContent` unmarshals into `map[string]interface{}`
/// and the Agent refuses such a file outright.
pub fn load(contents: &str) -> Result<Value> {
    let mut builder = Builder::default();
    for result in Parser::new_from_str(contents) {
        let (event, _) = result.context("parse YAML event")?;
        // `yaml.Unmarshal` stops at the first document and never tokenizes what follows,
        // so a later document that does not parse must not fail the file.
        let end_of_document = matches!(event, Event::DocumentEnd);
        builder.push(event)?;
        if end_of_document {
            break;
        }
    }
    let root = builder.finish()?;
    if !matches!(root, Value::Null | Value::Mapping(_)) {
        bail!("config file must be a YAML mapping");
    }
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

    // A valueless leaf is skipped rather than returned, so that it cannot mask the same
    // setting spelled another way. For a setting the schema knows, which is all a gate
    // asks about, `loadYamlInto` drops such a leaf instead of recording it, so a file
    // with both `a.b:` and `a: {b: true}` reads as true to the Agent whichever order it
    // happens to visit the two keys in.
    //
    // Every case-insensitive match is tried, not just the first: `loadYamlInto` lowercases
    // each key before merging, so `A: {b: }` and `a: {b: true}` collapse into one node in
    // which the valueless leaf is the one dropped. Stopping at the first match would let
    // whichever spelling happens to come first decide the answer.
    if let Some(value) = case_insensitive_matches(mapping, key).find(|value| value_is_set(value)) {
        return Some(value);
    }

    key.match_indices('.').find_map(|(dot, _)| {
        case_insensitive_matches(mapping, &key[..dot])
            .find_map(|child| lookup_dotted_key(child, &key[dot + 1..]))
    })
}

/// Whether a node counts as an explicit config value.
///
/// Mirrors `read_config_file.go`: a known leaf with no value (`setting_name:`) is
/// ignored and does not mark the key as configured.
fn value_is_set(value: &Value) -> bool {
    !matches!(value, Value::Null)
}

/// Every value in `mapping` whose key equals `key` ignoring ASCII case, exact spelling
/// first so that a file mixing spellings resolves the same way on every run.
fn case_insensitive_matches<'a, 'k>(
    mapping: &'a Mapping,
    key: &'k str,
) -> impl Iterator<Item = &'a Value> + use<'a, 'k> {
    let exact = mapping.get(key);
    exact
        .into_iter()
        .chain(mapping.iter().filter_map(move |(k, v)| {
            k.as_str()
                .filter(|segment| *segment != key && segment.eq_ignore_ascii_case(key))
                .map(|_| v)
        }))
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
    // `cast.ToStringE` formats a float64 with `strconv.FormatFloat(v, 'f', -1, 64)`: the
    // shortest form that round-trips, always positional. Rust's `Display` agrees, and
    // unlike a cast to i64 it does not saturate at i64::MAX for a value such as `1e20`.
    match number.as_f64() {
        // The two spell the infinities differently; both spell NaN `NaN`.
        Some(value) if value.is_infinite() && value.is_sign_negative() => "-Inf".to_owned(),
        Some(value) if value.is_infinite() => "+Inf".to_owned(),
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
        "timestamp" => tagged_timestamp(text),
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
///
/// The widening is not universal. `resolve`'s deferred check matches only the `int` and
/// `int64` cases, so a magnitude that needed `ParseUint` (above `i64::MAX`) falls through
/// to `failf`: `!!float 18446744073709551615` aborts `Unmarshal`.
fn tagged_float(text: &str) -> Result<Value> {
    let Value::Number(number) = plain_scalar_to_value(text) else {
        bail!("cannot decode `{text}` as !!float");
    };
    if number.is_f64() {
        return Ok(Value::Number(number));
    }
    if number.as_i64().is_none() {
        bail!("cannot decode `{text}` as !!float");
    }
    let widened = number
        .as_f64()
        .with_context(|| format!("cannot decode `{text}` as !!float"))?;
    Ok(Value::Number(widened.into()))
}

/// yaml.v2 resolves `!!timestamp` to a `time.Time` but then hands an `interface{}` the
/// original text instead, for backward compatibility, and `interface{}` is what the
/// Agent decodes a config file into. The value therefore needs no conversion; what
/// matters is that a scalar which is not a timestamp fails `Unmarshal`, so the Agent
/// rejects the whole file and no gate in it should be trusted either.
fn tagged_timestamp(text: &str) -> Result<Value> {
    if !is_timestamp(text) {
        bail!("cannot decode `{text}` as !!timestamp");
    }
    Ok(Value::String(text.to_owned()))
}

/// Whether a scalar is a timestamp, the way yaml.v2's `parseTimestamp` decides.
///
/// yaml.v2 tries four Go layouts and gives up when none of them parses:
///
/// ```text
/// 2006-1-2                         2006-1-2 15:4:5.999999999
/// 2006-1-2T15:4:5.999999999Z07:00  2006-1-2t15:4:5.999999999Z07:00
/// ```
///
/// The year is exactly four digits and every other field is one or two. `time.Parse`
/// also range-checks each field, so `2015-13-01` and `2015-02-29` are not timestamps.
fn is_timestamp(text: &str) -> bool {
    let Some((year, rest)) = take_number(text.as_bytes(), 4, 4) else {
        return false;
    };
    let Some((month, rest)) = take_byte(rest, b'-').and_then(|rest| take_number(rest, 1, 2)) else {
        return false;
    };
    let Some((day, rest)) = take_byte(rest, b'-').and_then(|rest| take_number(rest, 1, 2)) else {
        return false;
    };
    if !(1..=12).contains(&month) || !(1..=days_in_month(year, month)).contains(&day) {
        return false;
    }
    match rest {
        // `2006-1-2`
        [] => true,
        // `2006-1-2 15:4:5.999999999`, which carries no zone.
        [b' ', time @ ..] => matches!(take_time(time), Some([])),
        // `2006-1-2T15:4:5.999999999Z07:00`, where the zone is not optional.
        [b'T' | b't', time @ ..] => take_time(time).is_some_and(is_zone),
        _ => false,
    }
}

/// `15:4:5.999999999`: one or two digits per field, then an optional fraction that needs
/// at least one digit once its separator is there. Returns whatever follows the time.
///
/// Go's `parseNanoseconds` takes a comma as readily as a period, and accepts more digits
/// than the layout asks for, so `10:00:00,5` is as valid as `10:00:00.5`.
fn take_time(bytes: &[u8]) -> Option<&[u8]> {
    let (hour, rest) = take_number(bytes, 1, 2)?;
    let (minute, rest) = take_number(take_byte(rest, b':')?, 1, 2)?;
    let (second, rest) = take_number(take_byte(rest, b':')?, 1, 2)?;
    if hour > 23 || minute > 59 || second > 59 {
        return None;
    }
    match rest {
        [b'.' | b',', fraction @ ..] => {
            let width = fraction.iter().take_while(|b| b.is_ascii_digit()).count();
            (width > 0).then(|| &fraction[width..])
        }
        rest => Some(rest),
    }
}

/// `Z07:00`: a literal `Z`, or a sign and a two-digit `hh:mm`. Go rejects a lowercase
/// `z`, and deliberately bounds the offset with `>` rather than `>=` "as some people do
/// write offsets of 24 hours or 60 minutes".
fn is_zone(bytes: &[u8]) -> bool {
    let offset = match bytes {
        [b'Z'] => return true,
        [b'+' | b'-', offset @ ..] => offset,
        _ => return false,
    };
    let Some((hour, rest)) = take_number(offset, 2, 2) else {
        return false;
    };
    let Some((minute, [])) = take_byte(rest, b':').and_then(|rest| take_number(rest, 2, 2)) else {
        return false;
    };
    hour <= 24 && minute <= 60
}

fn days_in_month(year: u32, month: u32) -> u32 {
    let leap = year.is_multiple_of(4) && (!year.is_multiple_of(100) || year.is_multiple_of(400));
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 if leap => 29,
        2 => 28,
        _ => 0,
    }
}

fn take_byte(bytes: &[u8], expected: u8) -> Option<&[u8]> {
    match bytes {
        [first, rest @ ..] if *first == expected => Some(rest),
        _ => None,
    }
}

/// Reads between `min` and `max` digits, like Go's `getnum` with and without a fixed
/// width, and returns the value with the rest of the input.
fn take_number(bytes: &[u8], min: usize, max: usize) -> Option<(u32, &[u8])> {
    let width = bytes
        .iter()
        .take_while(|byte| byte.is_ascii_digit())
        .count()
        .min(max);
    if width < min {
        return None;
    }
    let value = bytes[..width]
        .iter()
        .fold(0, |number, digit| number * 10 + u32::from(digit - b'0'));
    Some((value, &bytes[width..]))
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

/// Resolves a numeric plain scalar the way yaml.v2's `resolve` does.
///
/// yaml.v2 dispatches on the first byte of the *untouched* text through `resolveTable`,
/// and only a `.`, a sign or a digit reaches a numeric path. That is why `1_` is the
/// integer 1 while `_1` stays a string: underscores are stripped only after the hint is
/// taken. Matching the order matters because `cast.ToBoolE` reads any non-zero number as
/// true, so a scalar that Go resolves to a number must not end up a string here.
fn plain_number(text: &str) -> Option<Value> {
    // `resolveMap` is consulted before any parsing, and covers the special floats.
    if let Some(number) = special_float(text) {
        return Some(Value::Number(number.into()));
    }
    match text.as_bytes().first()? {
        // Hint `.`: yaml.v2 goes straight to ParseFloat, with no shape filter.
        b'.' => finite_float(text),
        // Hint `D`/`S`: base-0 integers, then a `yamlStyleFloat`-shaped float.
        b'+' | b'-' | b'0'..=b'9' => {
            let plain: String = text.chars().filter(|c| *c != '_').collect();
            base0_int(&plain).or_else(|| {
                yaml_style_float(&plain)
                    .then(|| finite_float(&plain))
                    .flatten()
            })
        }
        // Every other first byte has no hint at all, so the scalar stays a string.
        _ => None,
    }
}

/// yaml.v2 falls through to a string when ParseFloat fails, and an overflow such as
/// `1e400` is a failure, so it is a string rather than an infinity.
fn finite_float(text: &str) -> Option<Value> {
    text.parse::<f64>()
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

/// `strconv.ParseInt`/`ParseUint` with base 0, which is how yaml.v2 resolves every
/// integer. Besides the `0x`/`0b`/`0o` prefixes, base 0 reads a bare leading zero as
/// C-style octal, so `010` is 8 and `0644` is 420. An invalid octal digit is not an
/// error: yaml.v2 falls through to its float fallback, which makes `08` the float 8.
fn base0_int(plain: &str) -> Option<Value> {
    let (negative, rest) = match plain.as_bytes() {
        [b'+', rest @ ..] => (false, rest),
        [b'-', rest @ ..] => (true, rest),
        rest => (false, rest),
    };
    let (base, digits) = match rest {
        [b'0', b'x' | b'X', digits @ ..] => (16, digits),
        [b'0', b'b' | b'B', digits @ ..] => (2, digits),
        [b'0', b'o' | b'O', digits @ ..] => (8, digits),
        // A lone `0` is just zero; any further digit makes it octal.
        [b'0', digits @ ..] if !digits.is_empty() => (8, digits),
        digits => (10, digits),
    };
    let digits = std::str::from_utf8(digits).ok()?;
    // Go reads the sign before the base prefix, so `0x-1` is not an integer, but
    // `from_str_radix` would accept it.
    if digits.is_empty() || digits.starts_with(['+', '-']) {
        return None;
    }
    if negative {
        if let Ok(n) = i64::from_str_radix(digits, base) {
            return Some(Value::Number(n.checked_neg()?.into()));
        }
        // ParseUint rejects a sign, so the only negative magnitude above i64::MAX that
        // Go accepts is 2^63 (e.g. -0x8000000000000000), which negates to i64::MIN.
        let magnitude = u64::from_str_radix(digits, base).ok()?;
        return (magnitude == i64::MIN.unsigned_abs()).then(|| Value::Number(i64::MIN.into()));
    }
    if let Ok(n) = i64::from_str_radix(digits, base) {
        return Some(Value::Number(n.into()));
    }
    u64::from_str_radix(digits, base)
        .ok()
        .map(|n| Value::Number(n.into()))
}

/// yaml.v2 gates its float fallback on `yamlStyleFloat`,
/// `^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$`. The filter is what keeps
/// Go's ParseFloat extensions (`Inf`, `NaN`, hex floats such as `0x1p-2`) out of YAML,
/// while still letting a digit-only scalar become a float once it has overflowed
/// `uint64`.
fn yaml_style_float(plain: &str) -> bool {
    let rest = match plain.as_bytes() {
        [b'+' | b'-', rest @ ..] => rest,
        rest => rest,
    };
    // `\.[0-9]+` or `[0-9]+(\.[0-9]*)?`
    let rest = match rest {
        [b'.', fraction @ ..] => match split_digits(fraction) {
            (0, _) => return false,
            (_, rest) => rest,
        },
        integer => match split_digits(integer) {
            (0, _) => return false,
            (_, [b'.', fraction @ ..]) => split_digits(fraction).1,
            (_, rest) => rest,
        },
    };
    // `([eE][-+]?[0-9]+)?`
    let rest = match rest {
        [b'e' | b'E', exponent @ ..] => {
            let exponent = match exponent {
                [b'+' | b'-', digits @ ..] => digits,
                digits => digits,
            };
            match split_digits(exponent) {
                (0, _) => return false,
                (_, rest) => rest,
            }
        }
        rest => rest,
    };
    rest.is_empty()
}

fn split_digits(bytes: &[u8]) -> (usize, &[u8]) {
    let width = bytes.iter().take_while(|b| b.is_ascii_digit()).count();
    (width, &bytes[width..])
}

fn mapping_from_pairs(pairs: HashMap<String, Value>) -> Value {
    let mut map = Mapping::new();
    for (key, value) in pairs {
        map.insert(Value::String(key), value);
    }
    Value::Mapping(map)
}

/// Converts a finished node into a mapping key.
///
/// Every key becomes a Go map key when the Agent unmarshals the file, and a sequence or
/// a mapping is not comparable, so yaml.v2 aborts with `invalid map key` rather than
/// inventing one. Returning an empty key instead would leave the rest of a file the
/// Agent rejects readable, and a gate could act on it.
fn scalar_as_key(value: Value) -> Result<String> {
    match value {
        Value::String(text) => Ok(text),
        Value::Bool(enabled) => Ok(enabled.to_string()),
        Value::Number(number) => Ok(number.to_string()),
        Value::Null => Ok("null".to_owned()),
        other => bail!("invalid map key: {other:?}"),
    }
}

/// yaml.v2's alias budget, from `decode.go`. Below these floors a document is too small
/// to be worth policing; above them the share of nodes that may come from alias
/// expansion tapers from 99% down to 10% as the document grows.
const ALIAS_COUNT_FLOOR: usize = 100;
const NODE_COUNT_FLOOR: usize = 1000;
const ALIAS_RATIO_RANGE_LOW: usize = 400_000;
const ALIAS_RATIO_RANGE_HIGH: usize = 4_000_000;

fn allowed_alias_ratio(node_count: usize) -> f64 {
    match node_count {
        count if count <= ALIAS_RATIO_RANGE_LOW => 0.99,
        count if count >= ALIAS_RATIO_RANGE_HIGH => 0.10,
        count => {
            let range = (ALIAS_RATIO_RANGE_HIGH - ALIAS_RATIO_RANGE_LOW) as f64;
            0.99 - 0.89 * ((count - ALIAS_RATIO_RANGE_LOW) as f64 / range)
        }
    }
}

fn count_nodes(value: &Value) -> usize {
    match value {
        Value::Mapping(mapping) => 1 + mapping.values().map(count_nodes).sum::<usize>(),
        Value::Sequence(items) => 1 + items.iter().map(count_nodes).sum::<usize>(),
        _ => 1,
    }
}

/// Event-driven document builder. Mapping keys go through a [`HashMap`] so duplicate
/// keys last-win, matching Go `yaml.Unmarshal`.
#[derive(Default)]
struct Builder {
    stack: Vec<Frame>,
    anchors: HashMap<usize, Value>,
    root: Option<Value>,
    /// Nodes materialized so far, and how many of them came from expanding an alias.
    node_count: usize,
    alias_node_count: usize,
}

enum Frame {
    Mapping {
        pairs: HashMap<String, Value>,
        pending: Pending,
        anchor: usize,
    },
    Sequence {
        items: Vec<Value>,
        anchor: usize,
    },
}

/// What the next node inside a mapping is.
#[derive(Default)]
enum Pending {
    /// A key.
    #[default]
    Key,
    /// The value for a key already read.
    Value(String),
    /// The mapping(s) to merge in for a `<<` key.
    Merge,
}

/// Whether a `<<` scalar with this style and tag is a merge key.
///
/// yaml.v2's `isMerge` accepts two spellings: a plain, untagged `<<`, or a `<<` under an
/// explicit `!!merge` tag, whose style then does not matter, so `!!merge "<<"` merges.
/// `!!merge` is not in `resolvableTag`, so the scalar keeps its text either way; only
/// the merge decision changes. Any other tag makes it an ordinary key, `!!str <<`
/// included.
fn can_be_merge_key(style: ScalarStyle, tag: Option<&Tag>) -> bool {
    match tag {
        None => style == ScalarStyle::Plain,
        Some(tag) => tag.is_yaml_core_schema() && tag.suffix == "merge",
    }
}

/// Applies a yaml.v2 `<<` merge in place.
///
/// yaml.v2 unmarshals the merged mapping over the one being built at the position the
/// `<<` appears, so a merge overwrites keys set before it and is overwritten by keys set
/// after it. A sequence is walked backwards because its earlier entries take precedence.
fn merge_into(pairs: &mut HashMap<String, Value>, value: Value) -> Result<()> {
    match value {
        Value::Mapping(mapping) => insert_all(pairs, mapping)?,
        Value::Sequence(items) => {
            for item in items.into_iter().rev() {
                let Value::Mapping(mapping) = item else {
                    bail!("map merge requires map or sequence of maps as the value");
                };
                insert_all(pairs, mapping)?;
            }
        }
        _ => bail!("map merge requires map or sequence of maps as the value"),
    }
    Ok(())
}

fn insert_all(pairs: &mut HashMap<String, Value>, mapping: Mapping) -> Result<()> {
    for (key, value) in mapping {
        pairs.insert(scalar_as_key(key)?, value);
    }
    Ok(())
}

impl Builder {
    fn push(&mut self, event: Event<'_>) -> Result<()> {
        match event {
            Event::Nothing
            | Event::StreamStart
            | Event::StreamEnd
            | Event::DocumentStart(_)
            | Event::DocumentEnd => {}
            Event::MappingStart(anchor, _) => self.stack.push(Frame::Mapping {
                pairs: HashMap::new(),
                pending: Pending::Key,
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
                self.attach(value, can_be_merge_key(style, tag.as_deref()))?;
            }
            Event::Alias(anchor) => {
                let value = self
                    .anchors
                    .get(&anchor)
                    .with_context(|| format!("unknown YAML alias anchor {anchor}"))?
                    .clone();
                // An alias materializes its whole anchored subtree. `attach` charges the
                // alias node itself, so the rest of the clone is charged here and
                // attributed to aliasing.
                let expanded = count_nodes(&value) - 1;
                self.charge_nodes(expanded, expanded)?;
                self.attach(value, false)?;
            }
        }
        Ok(())
    }

    fn finish_mapping(&mut self) -> Result<()> {
        let frame = self.stack.pop().context("unexpected YAML mapping end")?;
        let (pairs, anchor) = match frame {
            Frame::Mapping {
                pairs,
                pending: Pending::Key,
                anchor,
            } => (pairs, anchor),
            Frame::Mapping { .. } => bail!("YAML mapping ended before value for key"),
            Frame::Sequence { .. } => bail!("YAML mapping end without matching start"),
        };
        self.attach_anchored(mapping_from_pairs(pairs), anchor)
    }

    fn finish_sequence(&mut self) -> Result<()> {
        let frame = self.stack.pop().context("unexpected YAML sequence end")?;
        let (items, anchor) = match frame {
            Frame::Sequence { items, anchor } => (items, anchor),
            Frame::Mapping { .. } => bail!("YAML sequence end without matching start"),
        };
        self.attach_anchored(Value::Sequence(Sequence::from(items)), anchor)
    }

    fn store_anchor(&mut self, anchor: usize, value: &Value) {
        if anchor != 0 {
            self.anchors.insert(anchor, value.clone());
        }
    }

    fn attach_anchored(&mut self, value: Value, anchor: usize) -> Result<()> {
        self.store_anchor(anchor, &value);
        self.attach(value, false)
    }

    /// Adds a finished node to the collection being built. `merge_candidate` answers the
    /// half of yaml.v2's `isMerge` that the value alone cannot: whether the node's style
    /// and tag allow it to be a `<<` merge key.
    fn attach(&mut self, value: Value, merge_candidate: bool) -> Result<()> {
        self.charge_nodes(1, 0)?;
        match self.stack.last_mut() {
            None => self.root = Some(value),
            Some(Frame::Mapping { pairs, pending, .. }) => match std::mem::take(pending) {
                Pending::Key => {
                    *pending = if merge_candidate && value == Value::String("<<".to_owned()) {
                        Pending::Merge
                    } else {
                        Pending::Value(scalar_as_key(value)?)
                    };
                }
                Pending::Value(key) => {
                    pairs.insert(key, value);
                }
                Pending::Merge => merge_into(pairs, value)?,
            },
            Some(Frame::Sequence { items, .. }) => items.push(value),
        }
        Ok(())
    }

    /// Charges `total` newly materialized nodes, `from_alias` of which came from an
    /// alias, and enforces yaml.v2's alias budget.
    ///
    /// yaml.v2 rejects a document whose decoding is dominated by alias expansion, which
    /// is what stops a "billion laughs" file. This loader clones each anchored subtree
    /// eagerly, so without the same budget a few lines of YAML would exhaust memory and
    /// take procmgr down before `load` could return.
    fn charge_nodes(&mut self, total: usize, from_alias: usize) -> Result<()> {
        self.node_count += total;
        self.alias_node_count += from_alias;
        if self.alias_node_count > ALIAS_COUNT_FLOOR
            && self.node_count > NODE_COUNT_FLOOR
            && self.alias_node_count as f64 / self.node_count as f64
                > allowed_alias_ratio(self.node_count)
        {
            bail!("YAML document contains excessive aliasing");
        }
        Ok(())
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

    /// The Agent unmarshals a config file into `map[string]interface{}`, so a scalar or
    /// sequence root fails to load rather than reading as an empty config.
    #[test]
    fn non_mapping_document_roots_are_rejected() {
        for yaml in ["just a scalar\n", "- a\n- b\n", "123\n", "''\n"] {
            assert!(load(yaml).is_err(), "{yaml:?}");
        }
        // An explicitly null document still unmarshals into a nil map without error.
        for yaml in ["null\n", "~\n"] {
            assert_eq!(load(yaml).unwrap(), Value::Null, "{yaml:?}");
        }
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

    /// `yaml.Unmarshal` never reads past the first document, so the Agent loads this file
    /// and the gate must not see a parse error the Agent never hits.
    #[test]
    fn later_document_is_not_parsed_at_all() {
        assert_eq!(
            dotted(
                "process_config:\n  enabled: true\n---\n{\n",
                "process_config.enabled"
            ),
            Some(Value::Bool(true))
        );
        assert_eq!(
            dotted(
                "process_config:\n  enabled: true\n---\nenabled: !!int 1.0\n",
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

    /// `isMerge` also accepts an explicit `!!merge` tag, and then the scalar's style no
    /// longer matters, so a quoted `<<` merges too.
    #[test]
    fn explicitly_tagged_merge_keys_merge() {
        let base = "base: &base\n  enabled: true\n";
        for key in ["!!merge <<", "!!merge \"<<\""] {
            assert_eq!(
                dotted(
                    &format!("{base}process_config:\n  {key}: *base\n"),
                    "process_config.enabled"
                ),
                Some(Value::Bool(true)),
                "{key}"
            );
        }
    }

    /// yaml.v2's `isMerge` requires a plain, untagged scalar, so a quoted or `!!str`
    /// `<<` is an ordinary key and must neither merge nor fail the file.
    #[test]
    fn only_a_plain_merge_key_merges() {
        for yaml in [
            "base: &base\n  enabled: true\nprocess_config:\n  \"<<\": literal\n",
            "base: &base\n  enabled: true\nprocess_config:\n  !!str <<: literal\n",
        ] {
            assert_eq!(
                dotted(yaml, "process_config.<<"),
                Some(Value::String("literal".into())),
                "{yaml:?}"
            );
            assert_eq!(dotted(yaml, "process_config.enabled"), None, "{yaml:?}");
        }
    }

    /// yaml.v2 unmarshals a merge over the keys read so far, so `<<` overwrites what
    /// precedes it and loses to what follows it.
    #[test]
    fn merge_precedence_follows_document_order() {
        let base = "base: &base\n  enabled: true\n";
        assert_eq!(
            dotted(
                &format!("{base}process_config:\n  enabled: false\n  <<: *base\n"),
                "process_config.enabled"
            ),
            Some(Value::Bool(true))
        );
        assert_eq!(
            dotted(
                &format!("{base}process_config:\n  <<: *base\n  enabled: false\n"),
                "process_config.enabled"
            ),
            Some(Value::Bool(false))
        );
    }

    /// A sequence or mapping used as a key is not comparable, so yaml.v2 rejects the
    /// whole file instead of coining a key for it.
    #[test]
    fn collection_mapping_keys_are_rejected() {
        assert!(load("process_config:\n  ? [a, b]\n  : enabled\n").is_err());
        assert!(load("process_config:\n  ? {a: 1}\n  : enabled\n").is_err());
        assert!(load("? [a, b]\n: enabled\n").is_err());
        // Every scalar kind is still a usable key.
        for key in ["enabled", "!!str enabled", "1", "true", "~"] {
            assert!(
                load(&format!("process_config:\n  {key}: true\n")).is_ok(),
                "{key}"
            );
        }
    }

    /// A merge whose value is not a mapping aborts `Unmarshal`, so it must fail here too.
    #[test]
    fn merge_key_requires_a_mapping() {
        assert!(load("process_config:\n  <<: literal\n").is_err());
        assert!(load("process_config:\n  <<: [1, 2]\n").is_err());
    }

    /// A few lines of nested aliases expand to more nodes than there is memory for.
    /// yaml.v2 refuses such a document, and so must this loader.
    #[test]
    fn alias_bombs_are_rejected() {
        let mut yaml = String::from("l0: &l0 [a, a, a, a, a, a, a, a, a]\n");
        for level in 1..12 {
            let children = vec![format!("*l{}", level - 1); 9].join(", ");
            yaml.push_str(&format!("l{level}: &l{level} [{children}]\n"));
        }
        assert!(load(&yaml).is_err());
    }

    /// The budget must leave an ordinary config that reuses a handful of anchors alone.
    #[test]
    fn ordinary_alias_reuse_is_allowed() {
        let mut yaml = String::from("base: &base\n  enabled: true\n");
        for index in 0..50 {
            yaml.push_str(&format!("check{index}:\n  <<: *base\n"));
        }
        assert_eq!(dotted(&yaml, "check49.enabled"), Some(Value::Bool(true)));
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

    /// For a setting the schema knows, `loadYamlInto` drops a valueless leaf instead of
    /// recording it, so the same setting spelled as a nested mapping still wins. Go
    /// visits the two keys in map order, but because the valueless one is never
    /// inserted, both orders agree.
    #[test]
    fn valueless_leaf_does_not_mask_a_nested_value() {
        for yaml in [
            "process_config.enabled:\nprocess_config:\n  enabled: true\n",
            "process_config:\n  enabled: true\nprocess_config.enabled:\n",
        ] {
            assert_eq!(
                dotted(yaml, "process_config.enabled"),
                Some(Value::Bool(true)),
                "{yaml:?}"
            );
        }
        assert_eq!(
            dotted(
                "process_config:\n  process_collection.enabled:\n  process_collection:\n    enabled: true\n",
                "process_config.process_collection.enabled"
            ),
            Some(Value::Bool(true))
        );
    }

    /// `loadYamlInto` lowercases every key before merging, so two spellings of one
    /// setting land in the same node and the valueless one is the one dropped. Verified
    /// against the Go loader: all four files below read as true.
    #[test]
    fn valueless_leaf_does_not_mask_another_spelling() {
        for yaml in [
            "PROCESS_CONFIG.ENABLED: true\nprocess_config.enabled:\n",
            "process_config.enabled:\nPROCESS_CONFIG.ENABLED: true\n",
            "PROCESS_CONFIG:\n  ENABLED: true\nprocess_config:\n  enabled:\n",
            "process_config:\n  enabled:\nPROCESS_CONFIG:\n  ENABLED: true\n",
        ] {
            assert_eq!(
                dotted(yaml, "process_config.enabled"),
                Some(Value::Bool(true)),
                "{yaml:?}"
            );
        }
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

    /// `resolve`'s deferred `!!float` check widens only `int`/`int64`, so the tag accepts
    /// an integer exactly while it fits `i64`. The gap is not intuitive: `u64::MAX` is
    /// rejected, yet a magnitude past `u64::MAX` overflows both integer parsers, reaches
    /// the `yamlStyleFloat` fallback and resolves as a float before the tag is checked.
    #[test]
    fn tagged_float_widens_only_a_signed_integer() {
        for (text, expected) in [
            ("9223372036854775807", 9.223372036854776e18),
            ("0x7FFFFFFFFFFFFFFF", 9.223372036854776e18),
            ("-9223372036854775808", -9.223372036854776e18),
            // Already a float by the time the tag is applied.
            ("-9223372036854775809", -9.223372036854776e18),
            ("18446744073709551616", 1.8446744073709552e19),
        ] {
            let yaml = format!("enabled: !!float {text}\n");
            assert_eq!(scalar(&yaml).as_f64(), Some(expected), "{text}");
        }

        for text in [
            "9223372036854775808",
            "18446744073709551615",
            "0xFFFFFFFFFFFFFFFF",
        ] {
            let yaml = format!("enabled: !!float {text}\n");
            assert!(load(&yaml).is_err(), "{text}");
        }
    }

    /// yaml.v2 hands a timestamp to an `interface{}` as its original text, so a valid
    /// `!!timestamp` needs no conversion, but an invalid one fails the whole file.
    #[test]
    fn timestamp_scalars_are_validated_not_converted() {
        for text in [
            "2015-01-01",
            "2015-1-2",
            "2015-01-1",
            "2016-02-29",
            "2000-02-29",
            "0000-02-29",
            "2015-01-01 10:00:00",
            "2015-1-2 3:4:5",
            "2015-01-01 10:00:00.25",
            "2015-01-01T10:00:00Z",
            "2015-01-01t10:00:00Z",
            "2015-01-01T23:59:59Z",
            "2015-01-01T10:00:00.123456789Z",
            // Go's parseNanoseconds takes a comma as readily as a period.
            "2015-01-01T10:00:00,5Z",
            "2015-01-01T10:00:00,123456789Z",
            "2015-01-01 10:00:00,25",
            "2015-01-01T10:00:00+01:00",
            "2015-01-01T10:00:00-05:30",
            // Go bounds a zone offset with `>`, so 24 hours and 60 minutes still parse.
            "2015-01-01T10:00:00+24:00",
            "2015-01-01T10:00:00+01:60",
        ] {
            assert_eq!(
                scalar(&format!("enabled: !!timestamp \"{text}\"\n")),
                Value::String(text.into()),
                "{text}"
            );
        }
        for text in [
            "",
            "nope",
            "1",
            // Out-of-range fields, including a February that is not a leap year.
            "2015-00-01",
            "2015-13-01",
            "2015-01-00",
            "2015-01-32",
            "2015-02-29",
            "1900-02-29",
            "2015-04-31",
            "2015-01-01T24:00:00Z",
            "2015-01-01T23:60:00Z",
            "2015-01-01T23:59:60Z",
            "2015-01-01T10:00:00+25:00",
            "2015-01-01T10:00:00+01:99",
            // The year is exactly four digits.
            "999-01-01",
            "02015-01-01",
            // The `T` form needs a zone, and the zone spelling is exact.
            "2015-01-01T10:00:00",
            "2015-01-01T10:00:00z",
            "2015-01-01T10:00:00+0100",
            "2015-01-01T10:00:00+01",
            // A fraction needs a digit, and nothing may trail the timestamp.
            "2015-01-01T10:00:00.Z",
            "2015-01-01 10:00:00.",
            "2015-01-01T10:00:00,Z",
            "2015-01-01 10:00:00,",
            "2015-01-01 ",
            " 2015-01-01",
            "2015-01-01Textra",
            "2015-01-01T10:00:00Zextra",
            "2015-01-01 10:00:00 +0000",
        ] {
            assert!(
                load(&format!("enabled: !!timestamp \"{text}\"\n")).is_err(),
                "{text}"
            );
        }
    }

    /// An untagged date is a timestamp to yaml.v2 too, and reaches `interface{}` as the
    /// same text, so it must stay a plain string here rather than become a number.
    #[test]
    fn plain_dates_stay_strings() {
        for text in ["2015-01-01", "2015-01-01T10:00:00Z", "2015-13-45"] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::String(text.into()),
                "{text}"
            );
        }
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

    /// yaml.v2 resolves integers with `strconv.ParseInt(s, 0, 64)`, so a bare leading
    /// zero is octal rather than decimal.
    #[test]
    fn integers_resolve_with_base_zero() {
        assert_eq!(scalar("enabled: 0\n").as_i64(), Some(0));
        assert_eq!(scalar("enabled: 00\n").as_i64(), Some(0));
        assert_eq!(scalar("enabled: 010\n").as_i64(), Some(8));
        assert_eq!(scalar("enabled: 0644\n").as_i64(), Some(420));
        assert_eq!(scalar("enabled: 02472256\n").as_i64(), Some(685230));
        assert_eq!(scalar("enabled: -010\n").as_i64(), Some(-8));
        // An invalid octal digit is not an error, it reaches the float fallback.
        assert_eq!(scalar("enabled: 08\n").as_f64(), Some(8.0));
        assert_eq!(scalar("enabled: 09\n").as_f64(), Some(9.0));
        // Underscores are dropped, but only after the first byte has picked the hint.
        assert_eq!(scalar("enabled: 1_000\n").as_i64(), Some(1000));
        assert_eq!(scalar("enabled: 0_10\n").as_i64(), Some(8));
        assert_eq!(scalar("enabled: 1_\n").as_i64(), Some(1));
        assert_eq!(scalar("enabled: _1\n"), Value::String("_1".into()));
    }

    /// An integer too large for `uint64` is a float in yaml.v2, not a string, and
    /// `GetBool` reads any non-zero float as true.
    #[test]
    fn integer_overflow_falls_back_to_a_float() {
        let value = scalar("enabled: 18446744073709551616\n");
        assert_eq!(value.as_f64(), Some(18446744073709551616.0));
        assert_eq!(value_as_bool(&value), Some(true));
        assert_eq!(
            scalar("enabled: -9223372036854775809\n").as_f64(),
            Some(-9223372036854775809.0)
        );
        assert_eq!(
            scalar("enabled: 99999999999999999999999\n").as_f64(),
            Some(1e23)
        );
    }

    #[test]
    fn floats_follow_the_yaml_style_float_shape() {
        assert_eq!(scalar("enabled: 1.\n").as_f64(), Some(1.0));
        assert_eq!(scalar("enabled: 1.e3\n").as_f64(), Some(1000.0));
        assert_eq!(scalar("enabled: .5\n").as_f64(), Some(0.5));
        assert_eq!(scalar("enabled: -.5\n").as_f64(), Some(-0.5));
        assert_eq!(scalar("enabled: 1e5\n").as_f64(), Some(100000.0));
        assert_eq!(scalar("enabled: +1e5\n").as_f64(), Some(100000.0));
        // Shapes yaml.v2 refuses: Go's ParseFloat extensions, a truncated exponent, an
        // empty or invalid radix prefix, and an overflowing exponent.
        for text in [
            "1e", "e5", "1.2.3", "0x1p-2", "Inf", "NaN", "1e400", "-1e400", "0x", "0b", "0o",
            "0o8", "0b12",
        ] {
            assert_eq!(
                scalar(&format!("enabled: {text}\n")),
                Value::String(text.into()),
                "{text}"
            );
        }
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

    /// `cast.ToStringE` runs a float64 through `FormatFloat(v, 'f', -1, 64)`, which has
    /// no exponent and no upper bound, so a large float must not be clamped to i64::MAX.
    #[test]
    fn large_floats_stringify_without_saturating() {
        for (yaml, expected) in [
            ("enabled: 1e20\n", "100000000000000000000"),
            ("enabled: 1e17\n", "100000000000000000"),
            ("enabled: 1e-7\n", "0.0000001"),
            ("enabled: 18446744073709551616\n", "18446744073709552000"),
            ("enabled: 123456789012345678901\n", "123456789012345680000"),
            ("enabled: 1.5\n", "1.5"),
            ("enabled: .inf\n", "+Inf"),
            ("enabled: -.inf\n", "-Inf"),
            ("enabled: .nan\n", "NaN"),
        ] {
            assert_eq!(
                scalar_as_string(&scalar(yaml)).as_deref(),
                Some(expected),
                "{yaml:?}"
            );
        }
    }
}
