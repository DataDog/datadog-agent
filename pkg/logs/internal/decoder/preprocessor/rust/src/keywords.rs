// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

use crate::tokens::Token;

pub static PATTERNS: &[&str] = &[
    // 2-char: Apm
    "AM", "PM",
    // 3-char: Month
    "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC",
    // 3-char: Day
    "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN",
    // 3-char: Zone
    "UTC", "GMT", "EST", "EDT", "CST", "CDT", "MST", "MDT", "PST", "PDT",
    "JST", "KST", "IST", "MSK", "CET", "BST", "HST", "HDT", "NST", "NDT",
    // 4-char: Zone
    "CEST", "NZST", "NZDT", "ACST", "ACDT", "AEST", "AEDT", "AWST", "AWDT",
    "AKST", "AKDT", "CHST", "CHDT",
    // 4-char: Severity
    "WARN", "CRIT",
    // 5-char: Severity
    "FATAL", "ERROR", "PANIC", "ALERT", "EMERG", "CRASH",
    // 6-char: Severity
    "SEVERE", "FAILED",
    // 7-char: Severity
    "WARNING", "CRASHED", "FAILURE", "TIMEOUT",
    // 8-char: Severity
    "CRITICAL", "DEADLOCK",
    // 9-char: Severity
    "EMERGENCY", "EXCEPTION",
];

pub(crate) static PATTERN_TOKENS: &[Token] = &[
    // 2-char: Apm
    Token::Apm, Token::Apm,
    // 3-char: Month
    Token::Month, Token::Month, Token::Month, Token::Month, Token::Month, Token::Month,
    Token::Month, Token::Month, Token::Month, Token::Month, Token::Month, Token::Month,
    // 3-char: Day
    Token::Day, Token::Day, Token::Day, Token::Day, Token::Day, Token::Day, Token::Day,
    // 3-char: Zone
    Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    // 4-char: Zone
    Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone, Token::Zone,
    // 4-char: Severity
    Token::Warn, Token::Critical,
    // 5-char: Severity
    Token::Fatal, Token::Error, Token::Panic, Token::Alert, Token::Emergency, Token::Crash,
    // 6-char: Severity
    Token::Severe, Token::Failure,
    // 7-char: Severity
    Token::Warn, Token::Crash, Token::Failure, Token::Timeout,
    // 8-char: Severity
    Token::Critical, Token::Deadlock,
    // 9-char: Severity
    Token::Emergency, Token::Exception,
];

const _: () = assert!(PATTERNS.len() == PATTERN_TOKENS.len());

pub(crate) fn pattern_token(pattern_id: usize) -> Token {
    PATTERN_TOKENS[pattern_id]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pattern_count() {
        assert_eq!(PATTERNS.len(), PATTERN_TOKENS.len());
        // 2 Apm + 12 Month + 7 Day + 20 Zone(3-char) + 13 Zone(4-char)
        // + 2 Sev(4-char) + 6 Sev(5-char) + 2 Sev(6-char)
        // + 4 Sev(7-char) + 2 Sev(8-char) + 2 Sev(9-char) = 72
        assert_eq!(PATTERNS.len(), 72);
    }

    #[test]
    fn test_no_duplicate_patterns() {
        let mut seen = std::collections::HashSet::new();
        for &p in PATTERNS {
            assert!(seen.insert(p), "duplicate pattern: {p}");
        }
    }

    #[test]
    fn test_all_patterns_have_valid_tokens() {
        for (i, &token) in PATTERN_TOKENS.iter().enumerate() {
            assert_ne!(
                token,
                Token::End,
                "pattern {} ({}) maps to End",
                i,
                PATTERNS[i]
            );
        }
    }
}
