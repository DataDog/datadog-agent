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

/// Plain-scalar coercion aligned with Go yaml.v2 (YAML 1.1 bool/null spellings).
fn plain_scalar_to_value(text: &str) -> Value {
    match text.to_ascii_lowercase().as_str() {
        "" | "~" | "null" => Value::Null,
        "true" | "yes" | "on" | "y" => Value::Bool(true),
        "false" | "no" | "off" | "n" => Value::Bool(false),
        _ => text
            .parse::<i64>()
            .map(|n| Value::Number(n.into()))
            .unwrap_or_else(|_| Value::String(text.to_owned())),
    }
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
}
