use dd_sds::{
    Labels, ProximityKeywordsConfig, RegexRuleConfig, RootRuleConfig, SecondaryValidator,
    Suppressions,
};
use serde_json::json;
use shlib_core::Config;

use crate::payload::Match;

use super::{Scanner, ScanningRule};

/// Deserializes the scanning rules from an instance config, like the check.
fn rules_from_instance(instance_yaml: &str) -> Vec<ScanningRule> {
    let config = Config::from_str(instance_yaml).expect("failed to parse instance config");
    config
        .get("scanning_rules")
        .expect("failed to read scanning_rules from instance config")
}

/// Builds a `Scanner` from a full instance config.
fn scanner_from_instance(instance_yaml: &str) -> Scanner {
    Scanner::new(&rules_from_instance(instance_yaml)).expect("failed to build scanner")
}

// --- config parsing -------------------------------------------------------

#[test]
fn builds_scanner_from_all_rule_kinds() {
    let rules = rules_from_instance(
        r#"
scanning_rules:
  - id: rule-included
    pattern: '\d{6}'
    proximity_keywords:
      look_ahead_character_count: 30
      included_keywords: ['token']
  - id: rule-excluded
    pattern: '\d{6}'
    proximity_keywords:
      look_ahead_character_count: 30
      excluded_keywords: ['test']
  - id: rule-suppression
    precedence: Specific
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
    suppressions:
      ends_with: ['@example.com']
      exact_match: ['ignore@corp.io']
  - id: rule-luhn
    pattern: '\d{16}'
    validator:
      type: LuhnChecksum
"#,
    );

    let scanner = Scanner::new(&rules).expect("scanner builds from every rule kind");

    // Scanner keeps the rule ids in declaration order.
    assert_eq!(
        scanner.rule_ids,
        [
            "rule-included",
            "rule-excluded",
            "rule-suppression",
            "rule-luhn"
        ]
    );

    // Every rule, fully parsed. `RootRuleConfig::new` fills the defaults
    // (match_action None, scope all, precedence Specific, empty labels).
    let expected = vec![
        // included proximity keyword
        ScanningRule {
            id: "rule-included".to_string(),
            config: RootRuleConfig::new(RegexRuleConfig {
                pattern: r"\d{6}".to_string(),
                proximity_keywords: Some(ProximityKeywordsConfig {
                    look_ahead_character_count: 30,
                    included_keywords: vec!["token".to_string()],
                    excluded_keywords: vec![],
                }),
                validator: None,
                labels: Labels::empty(),
                pattern_capture_groups: None,
            }),
        },
        // excluded proximity keyword
        ScanningRule {
            id: "rule-excluded".to_string(),
            config: RootRuleConfig::new(RegexRuleConfig {
                pattern: r"\d{6}".to_string(),
                proximity_keywords: Some(ProximityKeywordsConfig {
                    look_ahead_character_count: 30,
                    included_keywords: vec![],
                    excluded_keywords: vec!["test".to_string()],
                }),
                validator: None,
                labels: Labels::empty(),
                pattern_capture_groups: None,
            }),
        },
        // ends_with / exact_match suppressions
        ScanningRule {
            id: "rule-suppression".to_string(),
            config: RootRuleConfig::new(RegexRuleConfig {
                pattern: r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+".to_string(),
                proximity_keywords: None,
                validator: None,
                labels: Labels::empty(),
                pattern_capture_groups: None,
            })
            .suppressions(Suppressions {
                starts_with: vec![],
                ends_with: vec!["@example.com".to_string()],
                exact_match: vec!["ignore@corp.io".to_string()],
            }),
        },
        // secondary validator
        ScanningRule {
            id: "rule-luhn".to_string(),
            config: RootRuleConfig::new(RegexRuleConfig {
                pattern: r"\d{16}".to_string(),
                proximity_keywords: None,
                validator: Some(SecondaryValidator::LuhnChecksum),
                labels: Labels::empty(),
                pattern_capture_groups: None,
            }),
        },
    ];
    assert_eq!(rules, expected);
}

// --- scanning behaviour ---------------------------------------------------

#[test]
fn scan_happy_path() {
    // Deserialize the scanning rules through the full instance config, exactly
    // as `CheckConfig::from_instance` does, then scan.
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
    suppressions:
      ends_with: ['@example.com']
scan_data: []
"#,
    );

    let data = json!({
        "email": [
            "alice@example.com",
            "bob@gmail.com",
            "carol@corp.io"
        ]
    });

    let matches = scanner.scan(data).expect("failed to scan data");

    // `alice@example.com` is suppressed; the other two rows match.
    assert_eq!(
        matches,
        vec![Match {
            rule_id: "email".to_string(),
            column_name: "email".to_string(),
            count_matched_rows: 2,
        }]
    );
}

#[test]
fn scan_requires_included_keyword() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: token
    pattern: '\d{6}'
    proximity_keywords:
      look_ahead_character_count: 30
      included_keywords: ['token']
scan_data: []
"#,
    );

    let data = json!({
        "note": [
            "token 111111",
            "999999"
        ]
    });

    let matches = scanner.scan(data).expect("failed to scan data");

    // Only the row with the `token` keyword nearby matches.
    assert_eq!(
        matches,
        vec![Match {
            rule_id: "token".to_string(),
            column_name: "note".to_string(),
            count_matched_rows: 1,
        }]
    );
}

#[test]
fn scan_excludes_by_keyword() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: code
    pattern: '\d{6}'
    proximity_keywords:
      look_ahead_character_count: 30
      excluded_keywords: ['test']
scan_data: []
"#,
    );

    let data = json!({
        "code": [
            "secret 222222",
            "test 333333"
        ]
    });

    let matches = scanner.scan(data).expect("failed to scan data");

    // The row preceded by the `test` keyword is excluded.
    assert_eq!(
        matches,
        vec![Match {
            rule_id: "code".to_string(),
            column_name: "code".to_string(),
            count_matched_rows: 1,
        }]
    );
}

#[test]
fn scan_counts_row_once_with_multiple_matches() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
scan_data: []
"#,
    );

    // A single row holds two emails: both match the rule, but they share the
    // same row path, so the row is counted once.
    let data = json!({ "email": ["alice@corp.io and bob@corp.io"] });

    let matches = scanner.scan(data).expect("failed to scan data");

    assert_eq!(
        matches,
        vec![Match {
            rule_id: "email".to_string(),
            column_name: "email".to_string(),
            count_matched_rows: 1,
        }]
    );
}

#[test]
fn scan_filters_by_luhn_checksum() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: credit-card
    pattern: '\d{16}'
    validator:
      type: LuhnChecksum
scan_data: []
"#,
    );

    let data = json!({
        "card": [
            "4242424242424242",
            "4242424242424241"
        ]
    });

    let matches = scanner.scan(data).expect("failed to scan data");

    // Only the Luhn-valid number is kept.
    assert_eq!(
        matches,
        vec![Match {
            rule_id: "credit-card".to_string(),
            column_name: "card".to_string(),
            count_matched_rows: 1,
        }]
    );
}

#[test]
fn scan_without_matches_is_empty() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
scan_data: []
"#,
    );

    let data = json!({ "name": ["alice", "bob"] });

    let matches = scanner.scan(data).expect("failed to scan data");

    assert!(matches.is_empty());
}

#[test]
fn scan_preserves_column_name_with_brackets() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
scan_data: []
"#,
    );

    // A quoted DB column can itself contain brackets, so the scanner path is
    // `foo[bar][0]`; only the trailing row subscript should be stripped.
    let data = json!({ "foo[bar]": ["alice@corp.io"] });

    let matches = scanner.scan(data).expect("failed to scan data");

    assert_eq!(
        matches,
        vec![Match {
            rule_id: "email".to_string(),
            column_name: "foo[bar]".to_string(),
            count_matched_rows: 1,
        }]
    );
}

#[test]
fn scan_preserves_column_name_with_dots() {
    let scanner = scanner_from_instance(
        r#"
task_id: task-1
scanning_rules:
  - id: email
    pattern: '[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]+'
scan_data: []
"#,
    );

    // A quoted DB column can contain dots. The column is the leading path field,
    // so it survives verbatim even though `.` is the Path segment separator.
    let data = json!({ "first.last": ["alice@corp.io", "bob@corp.io"] });

    let matches = scanner.scan(data).expect("failed to scan data");

    assert_eq!(
        matches,
        vec![Match {
            rule_id: "email".to_string(),
            column_name: "first.last".to_string(),
            count_matched_rows: 2,
        }]
    );
}
