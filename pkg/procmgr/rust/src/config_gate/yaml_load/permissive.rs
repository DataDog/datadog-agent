// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Permissive [`serde_yaml`] fallback: duplicate mapping keys last-win via [`HashMap`].

use std::collections::HashMap;

use anyhow::Result;
use serde::Deserialize;
use serde_yaml::{Mapping, Number, Sequence, Value};

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum PermissiveValue {
    Null,
    Bool(bool),
    Number(Number),
    String(String),
    Sequence(Vec<PermissiveValue>),
    Mapping(HashMap<String, PermissiveValue>),
}

pub(super) fn load(contents: &str) -> Result<Value> {
    let root: PermissiveValue = serde_yaml::from_str(contents)?;
    Ok(to_value(root))
}

fn to_value(value: PermissiveValue) -> Value {
    match value {
        PermissiveValue::Null => Value::Null,
        PermissiveValue::Bool(enabled) => Value::Bool(enabled),
        PermissiveValue::Number(number) => Value::Number(number),
        PermissiveValue::String(text) => Value::String(text),
        PermissiveValue::Sequence(items) => {
            Value::Sequence(Sequence::from_iter(items.into_iter().map(to_value)))
        }
        PermissiveValue::Mapping(items) => {
            let mut map = Mapping::new();
            for (key, item) in items {
                map.insert(Value::String(key), to_value(item));
            }
            Value::Mapping(map)
        }
    }
}
