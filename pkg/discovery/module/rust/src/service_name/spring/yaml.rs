// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use saphyr_parser::{Event, Parser};
use std::io::Read;

/// State machine for navigating YAML events to find a target key path.
enum State {
    /// Consuming preamble events until the root MappingStart.
    Preamble,
    /// Inside a mapping, expecting a key event.
    Key,
    /// Final key segment matched; next event is the target value.
    Value,
    /// Intermediate key segment matched; next event must be MappingStart.
    Descend,
    /// Skipping a non-matching key's value, or a complex key and its value.
    /// `nesting` tracks depth within a compound structure being skipped.
    Skip { nesting: u32, count: SkipCount },
}

/// Number of top-level YAML values remaining to skip.
/// Only two variants exist — a zero-count skip is impossible by construction.
enum SkipCount {
    /// Skip one value (the value of a non-matching simple key).
    Value,
    /// Skip a complex key's subtree, then its value.
    KeyAndValue,
}

impl SkipCount {
    /// Consume one item. Returns `Some` if more items remain, `None` if done.
    fn decrement(self) -> Option<Self> {
        match self {
            Self::KeyAndValue => Some(Self::Value),
            Self::Value => None,
        }
    }
}

/// Searches for a dot-separated key path (e.g. "spring.application.name") in
/// YAML and returns its scalar value. Uses a streaming event parser to avoid
/// building a full DOM tree.
///
/// Only returns the *first* occurrence of a matching key at each level. If the
/// YAML contains a duplicate key (e.g. `spring: 1` before `spring: {app: ...}`),
/// the parser matches the first one and won't reach the second.
pub fn parse_yaml<R: Read>(mut reader: R, target_key: &str) -> Option<String> {
    let mut content = String::new();
    reader.read_to_string(&mut content).ok()?;

    let segments: Vec<&str> = target_key.split('.').collect();
    let parser = Parser::new_from_str(&content);
    let mut depth: usize = 0;
    let mut state = State::Preamble;
    // Safety limit: the event count is bounded by input size (capped at
    // 1 MiB by MAX_PARSE_FILE_SIZE), but guard against parser bugs that
    // could produce infinite events.
    let max_events = content.len().saturating_mul(10).max(100);

    // Each YAML event drives a state transition. The parser walks a
    // path like "spring" → "application" → "name" by matching key
    // scalars at increasing depth and skipping unrelated entries.
    for result in parser.take(max_events) {
        let (event, _) = result.ok()?;
        state = match state {
            // Wait for the root mapping. StreamStart/DocumentStart are
            // boilerplate; the first real structure must be a mapping.
            State::Preamble => match event {
                Event::StreamStart | Event::DocumentStart(_) => State::Preamble,
                Event::MappingStart(..) => State::Key,
                _ => return None,
            },
            // Inside a mapping, expecting a key. A YAML mapping emits
            // events as: MappingStart, key, value, key, value, …, MappingEnd.
            // We're positioned at a "key" slot.
            State::Key => match event {
                // Exhausted all keys without a match.
                Event::MappingEnd => return None,
                // Key matches the current path segment.
                Event::Scalar(ref key, ..) if segments.get(depth) == Some(&key.as_ref()) => {
                    if depth == segments.len() - 1 {
                        // Last segment — next event is the target value.
                        State::Value
                    } else {
                        // Intermediate segment — next event must open a
                        // nested mapping so we can continue matching.
                        State::Descend
                    }
                }
                // Non-matching simple key: skip its value (1 item).
                Event::Scalar(..) | Event::Alias(..) => State::Skip {
                    nesting: 0,
                    count: SkipCount::Value,
                },
                // Non-matching complex key (mapping/sequence used as a key):
                // skip the key's own subtree (nesting=1) plus its value,
                // so we skip two top-level items (KeyAndValue).
                Event::MappingStart(..) | Event::SequenceStart(..) => State::Skip {
                    nesting: 1,
                    count: SkipCount::KeyAndValue,
                },
                _ => return None,
            },
            // The final key segment matched; consume the value.
            State::Value => match event {
                Event::Scalar(value, ..) => return Some(value.into_owned()),
                // Value is a mapping, sequence, or alias — not a string.
                _ => return None,
            },
            // An intermediate key segment matched; the value must be a
            // mapping so we can descend into it and keep matching.
            State::Descend => match event {
                Event::MappingStart(..) => {
                    depth += 1;
                    State::Key
                }
                // Value isn't a mapping (e.g. "spring: 1" when we need
                // spring.application.name) — can't descend further.
                _ => return None,
            },
            // Skip one or two top-level YAML items, depending on
            // whether we're skipping just a value (Value) or a complex
            // key's subtree followed by its value (KeyAndValue).
            //
            // Each item may be a scalar (consumed in one event) or a
            // compound structure (mapping/sequence) whose events must
            // be consumed until the matching end event.
            //
            // `nesting` tracks depth inside compound structures.
            // Starts at 0; incremented on MappingStart/SequenceStart,
            // decremented on their matching End. An item is finished
            // when nesting returns to 0.
            State::Skip { mut nesting, count } => {
                match event {
                    Event::MappingStart(..) | Event::SequenceStart(..) => nesting += 1,
                    Event::MappingEnd | Event::SequenceEnd if nesting > 0 => nesting -= 1,
                    Event::Scalar(..) | Event::Alias(..) => {}
                    _ => return None,
                }
                let done = nesting == 0;
                if done {
                    match count.decrement() {
                        // All items skipped; back to reading keys.
                        None => State::Key,
                        Some(count) => State::Skip { nesting, count },
                    }
                } else {
                    State::Skip { nesting, count }
                }
            }
        };
    }
    None
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_yaml_streaming() {
        let yaml = r#"
spring:
  application:
    name: myapp
partial:
  existent: 1
server:
  port: 8080
other:
  deeply:
    nested:
      value: test
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("myapp".to_string()));

        // Test property not found
        let result2 = parse_yaml(yaml, "non.existent.key");
        assert_eq!(result2, None);
        let result2 = parse_yaml(yaml, "partial.existent.key");
        assert_eq!(result2, None);

        // Test deeply nested property
        let result3 = parse_yaml(yaml, "other.deeply.nested.value");
        assert_eq!(result3, Some("test".to_string()));
    }

    #[test]
    fn test_parse_yaml_non_scalar_types() {
        let yaml = r#"
spring:
  application:
    name: myapp
    port: 8080
    enabled: true
    tags:
      - tag1
      - tag2
    config:
      timeout: 30
"#
        .as_bytes();

        // Scalar values are returned as-is (no type discrimination).
        let result = parse_yaml(yaml, "spring.application.port");
        assert_eq!(result, Some("8080".to_string()));

        let result = parse_yaml(yaml, "spring.application.enabled");
        assert_eq!(result, Some("true".to_string()));

        // Non-scalar values (arrays, objects) return None.
        let result = parse_yaml(yaml, "spring.application.tags");
        assert_eq!(result, None);

        let result = parse_yaml(yaml, "spring.application.config");
        assert_eq!(result, None);
    }

    #[test]
    fn test_parse_yaml_edge_cases() {
        // Test empty YAML
        let yaml = "".as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test YAML with only whitespace
        let yaml = "   \n  \n  ".as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test malformed YAML — the streaming parser returns the value if it
        // appears before the malformed section, since it doesn't validate the
        // entire document upfront.
        let yaml = r#"
spring:
  application:
    name: myapp
  invalid indentation
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("myapp".to_string()));

        // Malformed YAML where the target key cannot be reached returns None.
        let yaml = r#"
  invalid indentation
spring:
  application:
    name: myapp
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test empty string value
        let yaml = r#"
spring:
  application:
    name: ""
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("".to_string()));

        // Null is returned as the string "null" (no type discrimination).
        let yaml = r#"
spring:
  application:
    name: null
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("null".to_string()));
    }

    #[test]
    fn test_parse_yaml_target_key_not_first() {
        // Target key is last at every level of nesting.
        let yaml = r#"
logging:
  level: DEBUG
server:
  port: 8080
  context-path: /api
spring:
  profiles:
    active: prod
  datasource:
    url: jdbc:mysql://localhost/db
  application:
    description: My cool app
    name: myapp
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("myapp".to_string()));

        // Target key after compound values (arrays, nested objects) that must be skipped.
        let yaml = r#"
spring:
  application:
    tags:
      - tag1
      - tag2
    config:
      timeout: 30
      retries: 3
    name: after-compounds
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("after-compounds".to_string()));

        // Single key in the file, but not at the top.
        let yaml = r#"
unrelated:
  deeply:
    nested:
      structure:
        with:
          many: levels
spring:
  application:
    name: found-it
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("found-it".to_string()));
    }

    /// Billion laughs: N levels of anchor/alias nesting produce 10^N copies
    /// when a DOM parser expands aliases. With yaml-rust2's YamlLoader, 554
    /// bytes of YAML caused 30+ GB allocation and OOM.
    ///
    /// The streaming parser is immune because it never expands aliases — they
    /// appear as opaque Event::Alias tokens that are skipped or rejected.
    #[test]
    fn test_parse_yaml_billion_laughs() {
        // Build a billion-laughs payload: each level references the previous
        // anchor 10 times. 8 levels = 10^8 logical copies.
        fn make_billion_laughs(levels: usize) -> String {
            let mut lines = vec!["a0: &a0 \"AAAAAAAAAA\"".to_string()];
            for i in 1..=levels {
                let refs: Vec<String> = (0..10).map(|_| format!("*a{}", i - 1)).collect();
                lines.push(format!("a{i}: &a{i} [{}]", refs.join(", ")));
            }
            lines.push(format!("bomb: *a{levels}"));
            lines.join("\n")
        }

        // Pure billion-laughs payload (no matching key) — should return None instantly.
        let yaml = make_billion_laughs(8);
        let result = parse_yaml(yaml.as_bytes(), "spring.application.name");
        assert_eq!(result, None);

        // Target key exists alongside alias bombs — aliases are skipped,
        // the real value is returned.
        let yaml = format!(
            "{}\nspring:\n  application:\n    name: safe-value\n",
            make_billion_laughs(8)
        );
        let result = parse_yaml(yaml.as_bytes(), "spring.application.name");
        assert_eq!(result, Some("safe-value".to_string()));

        // Target value is itself an alias — returns None (not a scalar).
        let yaml = "anchor: &bomb \"kaboom\"\nspring:\n  application:\n    name: *bomb\n";
        let result = parse_yaml(yaml.as_bytes(), "spring.application.name");
        assert_eq!(result, None);
    }
}
