// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Event-driven YAML loader with Go yaml.v2 plain-scalar bool coercion.

use std::collections::HashMap;

use anyhow::{Context, Result, bail};
use saphyr_parser::{Event, Parser, ScalarStyle};
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

fn scalar_to_value(text: &str, style: ScalarStyle) -> Value {
    if style == ScalarStyle::Plain {
        plain_scalar_to_value(text)
    } else {
        Value::String(text.to_owned())
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

/// Integers first, then finite floats. Matches yaml.v2 unmarshalling into
/// `interface{}` so `cast.ToBoolE` can treat a non-zero number as true.
fn yaml_v2_plain_number(text: &str) -> Option<Value> {
    if let Ok(n) = text.parse::<i64>() {
        return Some(Value::Number(n.into()));
    }
    if !looks_like_yaml_11_float(text) {
        return None;
    }
    text.parse::<f64>()
        .ok()
        .filter(|n| n.is_finite())
        .map(|n| Value::Number(n.into()))
}

/// Digit-based decimal or scientific form (`1.0`, `1e0`). Rejects Rust-only
/// spellings such as `inf` that yaml.v2 would leave as strings.
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
        match event {
            Event::Nothing | Event::StreamStart | Event::StreamEnd | Event::DocumentEnd => {}
            Event::DocumentStart(_) => {
                self.root = None;
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
            Event::Scalar(text, style, anchor, _) => {
                let value = scalar_to_value(&text, style);
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
    fn plain_yaml_11_bool_coerced_at_load() {
        let root = load("enabled: yes\n").unwrap();
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
    fn rust_inf_spelling_stays_string() {
        let root = load("enabled: inf\n").unwrap();
        assert_eq!(root.get("enabled"), Some(&Value::String("inf".into())));
    }
}
