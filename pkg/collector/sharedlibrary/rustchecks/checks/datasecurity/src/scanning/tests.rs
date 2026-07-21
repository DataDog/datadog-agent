use dd_sds::{MatchAction, SecondaryValidator};
use serde_json::{json, Value};

use super::{Scanner, ScanningRule};

fn parse(yaml: &str) -> Vec<ScanningRule> {
    serde_yaml::from_str(yaml).expect("failed to deserialize scanning rules")
}

fn scanner(yaml: &str) -> Scanner {
    Scanner::new(&parse(yaml)).expect("failed to build scanner")
}

/// Scans `data` and returns the matches as JSON, so tests can assert against a
/// single `json!` literal.
fn scan_json(scanner: &Scanner, data: &Value) -> Value {
    let matches = scanner.scan(data).expect("failed to scan data");
    serde_json::to_value(matches).expect("failed to serialize matches")
}

// --- config parsing -------------------------------------------------------

#[test]
fn builds_scanner_from_all_rule_kinds() {
    let rules = parse(
        r#"
# included proximity keyword
- id: rule-included
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    included_keywords: ['token']
# excluded proximity keyword
- id: rule-excluded
  pattern: '\d{6}'
  proximity_keywords:
    look_ahead_character_count: 30
    excluded_keywords: ['test']
# precedence + suppressions (ends_with / exact_match)
- id: rule-suppression
  precedence: Specific
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
  suppressions:
    ends_with: ['@example.com']
    exact_match: ['ignore@corp.io']
# secondary validator
- id: rule-luhn
  pattern: '\d{16}'
  validator:
    type: LuhnChecksum
"#,
    );

    // Each rule kind parsed and kept its id, in order.
    let ids: Vec<&str> = rules.iter().map(|r| r.id.as_str()).collect();
    assert_eq!(
        ids,
        ["rule-included", "rule-excluded", "rule-suppression", "rule-luhn"]
    );
    
    // Spot-check the parsed config beyond the id.
    let included = rules[0].config.inner.proximity_keywords.as_ref().unwrap();
    assert_eq!(included.included_keywords, vec!["token".to_string()]);
    let excluded = rules[1].config.inner.proximity_keywords.as_ref().unwrap();
    assert_eq!(excluded.excluded_keywords, vec!["test".to_string()]);
    assert_eq!(
        rules[3].config.inner.validator,
        Some(SecondaryValidator::LuhnChecksum)
    );
    // `suppressions` is private on `RootRuleConfig`, so round-trip through
    // serde to confirm `ends_with` parsed.
    let suppression = serde_json::to_value(&rules[2].config).expect("serialize config");
    assert_eq!(suppression["suppressions"]["ends_with"], json!(["@example.com"]));
    // `match_action` defaults to `None` when omitted.
    assert_eq!(rules[0].config.match_action, MatchAction::None);

    Scanner::new(&rules).expect("scanner builds from every rule kind");
}

// --- scanning behaviour ---------------------------------------------------

#[test]
fn suppression_drops_matching_rows() {
    let scanner = scanner(
        r#"
- id: email
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
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
"#,
    );

    let data = json!({ "name": ["alice", "bob"] });
    assert_eq!(scan_json(&scanner, &data), json!([]));
}

#[test]
fn column_name_with_brackets_is_preserved() {
    let scanner = scanner(
        r#"
- id: email
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
"#,
    );

    // A quoted DB column can itself contain brackets, so the scanner path is
    // `foo[bar][0]`; only the trailing row subscript should be stripped.
    let data = json!({ "foo[bar]": ["alice@corp.io"] });
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "email", "column_name": "foo[bar]", "count_matched_rows": 1 }
        ])
    );
}

#[test]
fn column_name_with_dots_is_preserved() {
    let scanner = scanner(
        r#"
- id: email
  pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
"#,
    );

    // A quoted DB column can contain dots. The column is the leading path field,
    // so it survives verbatim even though `.` is the Path segment separator.
    let data = json!({ "first.last": ["alice@corp.io", "bob@corp.io"] });
    assert_eq!(
        scan_json(&scanner, &data),
        json!([
            { "rule_id": "email", "column_name": "first.last", "count_matched_rows": 2 }
        ])
    );
}
