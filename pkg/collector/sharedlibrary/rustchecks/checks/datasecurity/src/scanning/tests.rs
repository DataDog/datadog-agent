use dd_sds::{MatchAction, SecondaryValidator};
use serde_json::{json, Value};

use super::{Scanner, ScanningRule};

fn parse(yaml: &str) -> Vec<ScanningRule> {
    serde_yaml::from_str(yaml).expect("deserialize scanning rules")
}

fn scanner(yaml: &str) -> Scanner {
    Scanner::new(&parse(yaml)).expect("build scanner")
}

/// Scans `data` and returns the matches as JSON, so tests can assert against a
/// single `json!` literal.
fn scan_json(scanner: &Scanner, data: &Value) -> Value {
    let matches = scanner.scan(data).expect("scan");
    serde_json::to_value(matches).expect("serialize matches")
}

// --- config parsing -------------------------------------------------------

#[test]
fn parses_included_keyword_rule() {
    let rules = parse(
        r#"
- id: rule-included
  name: Token With Included Keyword
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    included_keywords: ['token']
    excluded_keywords: []
"#,
    );

    assert_eq!(rules.len(), 1);
    assert_eq!(rules[0].id, "rule-included");
    let keywords = rules[0]
        .config
        .inner
        .proximity_keywords
        .as_ref()
        .expect("proximity keywords parsed");
    assert_eq!(keywords.included_keywords, vec!["token".to_string()]);
    assert!(keywords.excluded_keywords.is_empty());
    // `match_action` defaults to `None` when omitted.
    assert_eq!(rules[0].config.match_action, MatchAction::None);
}

#[test]
fn parses_excluded_keyword_rule() {
    let rules = parse(
        r#"
- id: rule-excluded
  name: Token With Excluded Keyword
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    included_keywords: []
    excluded_keywords: ['test']
"#,
    );

    let keywords = rules[0]
        .config
        .inner
        .proximity_keywords
        .as_ref()
        .expect("proximity keywords parsed");
    assert_eq!(keywords.excluded_keywords, vec!["test".to_string()]);
}

#[test]
fn parses_suppression_rule() {
    let rules = parse(
        r#"
- id: rule-suppression
  name: Email With Suppression
  precedence: Specific
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
  suppressions:
    ends_with: ['@example.com']
    exact_match: ['ignore@corp.io']
"#,
    );

    // `suppressions` and `precedence` are private fields of dd-sds
    // `RootRuleConfig`; a successful build proves they were accepted and compiled.
    Scanner::new(&rules).expect("scanner builds with suppressions");
}

#[test]
fn parses_luhn_checksum_rule() {
    let rules = parse(
        r#"
- id: rule-luhn
  name: Credit Card With Luhn
  pattern: '\d{16}'
  validator:
    type: LuhnChecksum
"#,
    );

    assert_eq!(
        rules[0].config.inner.validator,
        Some(SecondaryValidator::LuhnChecksum)
    );
}

#[test]
fn builds_scanner_from_all_rule_kinds() {
    let rules = parse(
        r#"
- id: rule-included
  name: Token With Included Keyword
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    included_keywords: ['token']
- id: rule-excluded
  name: Token With Excluded Keyword
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    excluded_keywords: ['test']
- id: rule-suppression
  name: Email With Suppression
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
  suppressions:
    ends_with: ['@example.com']
- id: rule-luhn
  name: Credit Card With Luhn
  pattern: '\d{16}'
  validator:
    type: LuhnChecksum
"#,
    );

    Scanner::new(&rules).expect("scanner builds from every rule kind");
}

// --- scanning behaviour ---------------------------------------------------

#[test]
fn suppression_drops_matching_rows() {
    let scanner = scanner(
        r#"
- id: email
  name: Email
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
  suppressions:
    ends_with: ['@example.com']
"#,
    );

    let data = json!({
        "email": [
            "alice@example.com",
            "bob@gmail.com",
            "carol@corp.io"
        ]
    });

    // `alice@example.com` is suppressed; the other two rows match.
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "email", "column_name": "email", "count_matched_rows": 2 }
        ])
    );
}

#[test]
fn included_keyword_required_for_match() {
    let scanner = scanner(
        r#"
- id: token
  name: Six Digit Token
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    included_keywords: ['token']
"#,
    );

    let data = json!({
        "note": [
            "token 111111",
            "999999"
        ]
    });

    // Only the row with the `token` keyword nearby matches.
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "token", "column_name": "note", "count_matched_rows": 1 }
        ])
    );
}

#[test]
fn excluded_keyword_suppresses_match() {
    let scanner = scanner(
        r#"
- id: code
  name: Six Digit Code
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    excluded_keywords: ['test']
"#,
    );

    let data = json!({
        "code": [
            "secret 222222",
            "test 333333"
        ]
    });

    // The row preceded by the `test` keyword is excluded.
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "code", "column_name": "code", "count_matched_rows": 1 }
        ])
    );
}

#[test]
fn luhn_checksum_filters_invalid_numbers() {
    let scanner = scanner(
        r#"
- id: credit-card
  name: Credit Card
  pattern: '\d{16}'
  validator:
    type: LuhnChecksum
"#,
    );

    let data = json!({
        "card": [
            "4242424242424242",
            "4242424242424241"
        ]
    });

    // Only the Luhn-valid number is kept.
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "credit-card", "column_name": "card", "count_matched_rows": 1 }
        ])
    );
}

#[test]
fn no_matches_produce_empty() {
    let scanner = scanner(
        r#"
- id: email
  name: Email
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
"#,
    );

    let data = json!({ "name": ["alice", "bob"] });
    assert_eq!(scan_json(&scanner, &data), json!([]));
}
