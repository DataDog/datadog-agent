// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

use crate::tokens::{CharClass, Token, CHAR_CLASS_LUT, MAX_RUN, MAX_SPECIAL_TOKEN_LEN, TO_UPPER};

pub struct Tokenizer {
    max_eval_bytes: usize,
}

impl Tokenizer {
    pub fn new(max_eval_bytes: usize) -> Self {
        Tokenizer { max_eval_bytes }
    }

    pub fn tokenize(&self, input: &[u8]) -> (Vec<Token>, Vec<i32>) {
        let input = self.truncate(input);
        if input.is_empty() {
            return (Vec::new(), Vec::new());
        }
        let est_tokens = input.len() / 4 + 8;
        let mut tokens = Vec::with_capacity(est_tokens);
        let mut indices = Vec::with_capacity(est_tokens);
        self.scan(input, |tok, idx| {
            tokens.push(tok);
            indices.push(idx);
        });
        (tokens, indices)
    }

    /// Tokenize directly into caller-owned raw buffers. Returns count written,
    /// or usize::MAX if capacity is insufficient.
    pub fn tokenize_into(
        &self,
        input: &[u8],
        tokens_out: &mut [u8],
        indices_out: &mut [i32],
    ) -> usize {
        let input = self.truncate(input);
        if input.is_empty() {
            return 0;
        }
        let capacity = tokens_out.len().min(indices_out.len());
        let mut n: usize = 0;
        let mut overflow = false;
        self.scan(input, |tok, idx| {
            if n < capacity {
                tokens_out[n] = tok as u8;
                indices_out[n] = idx;
                n += 1;
            } else {
                overflow = true;
            }
        });
        if overflow { usize::MAX } else { n }
    }

    #[inline]
    fn truncate<'a>(&self, input: &'a [u8]) -> &'a [u8] {
        let mut max_bytes = input.len();
        if self.max_eval_bytes > 0 && self.max_eval_bytes < max_bytes {
            max_bytes = self.max_eval_bytes;
        }
        &input[..max_bytes]
    }

    #[inline]
    fn scan(&self, input: &[u8], mut emit: impl FnMut(Token, i32)) {
        let mut run: usize = 0;
        let first_byte = input[0];
        let mut last_class = CHAR_CLASS_LUT[first_byte as usize];

        let mut str_buf = [0u8; MAX_RUN];
        let mut str_len: usize = 0;
        if last_class == CharClass::Letter {
            str_buf[0] = TO_UPPER[first_byte as usize];
            str_len = 1;
        }

        for i in 1..input.len() {
            let byte = input[i];
            let current_class = CHAR_CLASS_LUT[byte as usize];

            if current_class != last_class {
                emit_token(
                    &mut emit,
                    last_class,
                    run,
                    (i - 1) as i32,
                    &str_buf,
                    str_len,
                );
                run = 0;
                str_len = 0;
                if current_class == CharClass::Letter {
                    str_buf[0] = TO_UPPER[byte as usize];
                    str_len = 1;
                }
            } else {
                run += 1;
                if current_class == CharClass::Letter && str_len < MAX_RUN {
                    str_buf[str_len] = TO_UPPER[byte as usize];
                    str_len += 1;
                }
            }

            last_class = current_class;
        }

        emit_token(
            &mut emit,
            last_class,
            run,
            (input.len() - 1) as i32,
            &str_buf,
            str_len,
        );
    }
}

#[inline]
fn emit_token(
    emit: &mut impl FnMut(Token, i32),
    class: CharClass,
    run: usize,
    idx: i32,
    str_buf: &[u8; MAX_RUN],
    str_len: usize,
) {
    let token_len = run + 1;

    match class {
        CharClass::Letter => {
            if token_len <= MAX_SPECIAL_TOKEN_LEN && str_len > 0 {
                if str_len == 1 {
                    match str_buf[0] {
                        b'T' => { emit(Token::T, idx); return; }
                        b'Z' => { emit(Token::Zone, idx); return; }
                        _ => {}
                    }
                } else if let Some(tok) = keyword_lookup(str_buf, str_len) {
                    emit(tok, idx - run as i32);
                    return;
                }
            }
            let r = run.min(MAX_RUN - 1);
            emit(token_from_offset(Token::C1, r), idx - run as i32);
        }
        CharClass::Digit => {
            let r = run.min(MAX_RUN - 1);
            emit(token_from_offset(Token::D1, r), idx - run as i32);
        }
        CharClass::Space => {
            emit(Token::Space, idx - run as i32);
        }
        CharClass::Symbol(sym) => {
            emit(sym, idx - run as i32);
        }
    }
}

#[inline]
fn token_from_offset(base: Token, offset: usize) -> Token {
    unsafe { std::mem::transmute::<u8, Token>(base as u8 + offset as u8) }
}

#[inline]
fn keyword_lookup(buf: &[u8; MAX_RUN], len: usize) -> Option<Token> {
    match len {
        2 => {
            if (buf[0] == b'A' || buf[0] == b'P') && buf[1] == b'M' {
                return Some(Token::Apm);
            }
        }
        3 => {
            let b = [buf[0], buf[1], buf[2]];
            match &b {
                b"JAN" | b"FEB" | b"MAR" | b"APR" | b"MAY" | b"JUN" | b"JUL" | b"AUG"
                | b"SEP" | b"OCT" | b"NOV" | b"DEC" => return Some(Token::Month),
                b"MON" | b"TUE" | b"WED" | b"THU" | b"FRI" | b"SAT" | b"SUN" => {
                    return Some(Token::Day)
                }
                b"UTC" | b"GMT" | b"EST" | b"EDT" | b"CST" | b"CDT" | b"MST" | b"MDT"
                | b"PST" | b"PDT" | b"JST" | b"KST" | b"IST" | b"MSK" | b"CET" | b"BST"
                | b"HST" | b"HDT" | b"NST" | b"NDT" => return Some(Token::Zone),
                _ => {}
            }
        }
        4 => {
            let b = [buf[0], buf[1], buf[2], buf[3]];
            match &b {
                b"WARN" => return Some(Token::Warn),
                b"CRIT" => return Some(Token::Critical),
                b"CEST" | b"NZST" | b"NZDT" | b"ACST" | b"ACDT" | b"AEST" | b"AEDT"
                | b"AWST" | b"AWDT" | b"AKST" | b"AKDT" | b"CHST" | b"CHDT" => {
                    return Some(Token::Zone)
                }
                _ => {}
            }
        }
        5 => {
            let b = [buf[0], buf[1], buf[2], buf[3], buf[4]];
            match &b {
                b"FATAL" => return Some(Token::Fatal),
                b"ERROR" => return Some(Token::Error),
                b"PANIC" => return Some(Token::Panic),
                b"ALERT" => return Some(Token::Alert),
                b"EMERG" => return Some(Token::Emergency),
                b"CRASH" => return Some(Token::Crash),
                _ => {}
            }
        }
        6 => {
            if &buf[..6] == b"SEVERE" {
                return Some(Token::Severe);
            }
            if &buf[..6] == b"FAILED" {
                return Some(Token::Failure);
            }
        }
        7 => {
            if &buf[..7] == b"WARNING" {
                return Some(Token::Warn);
            }
            if &buf[..7] == b"CRASHED" {
                return Some(Token::Crash);
            }
            if &buf[..7] == b"FAILURE" {
                return Some(Token::Failure);
            }
            if &buf[..7] == b"TIMEOUT" {
                return Some(Token::Timeout);
            }
        }
        8 => {
            if &buf[..8] == b"CRITICAL" {
                return Some(Token::Critical);
            }
            if &buf[..8] == b"DEADLOCK" {
                return Some(Token::Deadlock);
            }
        }
        9 => {
            if &buf[..9] == b"EMERGENCY" {
                return Some(Token::Emergency);
            }
            if &buf[..9] == b"EXCEPTION" {
                return Some(Token::Exception);
            }
        }
        _ => {}
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tokens::tokens_to_string;

    fn tok(input: &str) -> String {
        let tokenizer = Tokenizer::new(0);
        let (tokens, _) = tokenizer.tokenize(input.as_bytes());
        tokens_to_string(&tokens)
    }

    fn tok_with_indices(input: &str) -> (String, Vec<i32>) {
        let tokenizer = Tokenizer::new(0);
        let (tokens, indices) = tokenizer.tokenize(input.as_bytes());
        (tokens_to_string(&tokens), indices)
    }

    #[test]
    fn test_tokenizer_parity() {
        let cases: &[(&str, &str)] = &[
            ("", ""),
            (" ", " "),
            ("a", "C"),
            ("a       b", "C C"),
            ("a  \t \t b", "C C"),
            ("aaa", "CCC"),
            ("0", "D"),
            ("000", "DDD"),
            ("aa00", "CCDD"),
            ("abcd", "CCCC"),
            ("1234", "DDDD"),
            ("abc123", "CCCDDD"),
            (
                "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~",
                "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~",
            ),
            ("123-abc-[foo] (bar)", "DDD-CCC-[CCC] (CCC)"),
            ("Sun Mar 2PM EST", "DAY MTH DPM ZONE"),
            (
                "12-12-12T12:12:12.12T12:12Z123",
                "DD-DD-DDTDD:DD:DD.DDTDD:DDZONEDDD",
            ),
            ("amped", "CCCCC"),
            ("am!ped", "PM!CCC"),
            ("TIME", "CCCC"),
            ("T123", "TDDD"),
            ("ZONE", "CCCC"),
            ("Z0NE", "ZONEDCC"),
            (
                "abc!\u{1f4c0}\u{1f436}\u{1f4ca}123",
                "CCC!CCCCCCCCCCDDD",
            ),
            ("!!!$$$###", "!$#"),
            ("FATAL", "FATAL"),
            ("fatal", "FATAL"),
            ("Fatal", "FATAL"),
            ("ERROR", "ERROR"),
            ("PANIC", "PANIC"),
            ("ALERT", "ALERT"),
            ("SEVERE", "SEVERE"),
            ("WARN", "WARN"),
            ("WARNING", "WARN"),
            ("CRIT", "CRIT"),
            ("CRITICAL", "CRIT"),
            ("EMERG", "EMERG"),
            ("EMERGENCY", "EMERG"),
            ("EXCEPTION", "EXCEPTION"),
            ("CRASH", "CRASH"),
            ("CRASHED", "CRASH"),
            ("FAILED", "FAILURE"),
            ("FAILURE", "FAILURE"),
            ("DEADLOCK", "DEADLOCK"),
            ("TIMEOUT", "TIMEOUT"),
            ("EXCEPTIONS", "CCCCCCCCCC"),
            ("FATALIZER", "CCCCCCCCC"),
            ("[ERROR] something", "[ERROR] CCCCCCCCC"),
            ("FATAL: disk full", "FATAL: CCCC CCCC"),
        ];

        for &(input, expected) in cases {
            let actual = tok(input);
            assert_eq!(actual, expected, "input: {input:?}");
        }
    }

    #[test]
    fn test_max_char_run() {
        let (s, indices) = tok_with_indices("ABCDEFGHIJKLMNOP");
        assert_eq!(s, "CCCCCCCCCC");
        assert_eq!(indices, vec![0]);
    }

    #[test]
    fn test_max_digit_run() {
        let (s, indices) = tok_with_indices("0123456789012345");
        assert_eq!(s, "DDDDDDDDDD");
        assert_eq!(indices, vec![0]);
    }

    #[test]
    fn test_max_eval_bytes() {
        let tokenizer = Tokenizer::new(10);
        let (tokens, _) = tokenizer.tokenize(b"1234567890abcdefg");
        assert_eq!(tokens_to_string(&tokens), "DDDDDDDDDD");
    }

    #[test]
    fn test_max_eval_bytes_with_index() {
        let tokenizer = Tokenizer::new(10);
        let (tokens, indices) = tokenizer.tokenize(b"12-12-12T12:12:12.12T12:12Z123");
        assert_eq!(tokens_to_string(&tokens), "DD-DD-DDTD");
        assert_eq!(indices, vec![0, 2, 3, 5, 6, 8, 9]);
    }

    #[test]
    fn test_empty_input() {
        let tokenizer = Tokenizer::new(0);
        let (tokens, indices) = tokenizer.tokenize(b"");
        assert!(tokens.is_empty());
        assert!(indices.is_empty());
    }

    #[test]
    fn test_determinism() {
        let tokenizer = Tokenizer::new(0);
        let input = b"2024-01-15T10:30:45.123Z INFO [service-name] Request processed";
        let (t1, i1) = tokenizer.tokenize(input);
        let (t2, i2) = tokenizer.tokenize(input);
        assert_eq!(t1, t2);
        assert_eq!(i1, i2);
    }
}

#[cfg(test)]
mod proptests {
    use super::*;
    use crate::tokens::{tokens_to_string, CharClass, Token, CHAR_CLASS_LUT, MAX_RUN, MAX_SPECIAL_TOKEN_LEN, TO_UPPER};
    use proptest::prelude::*;

    fn reference_tokenize(input: &[u8]) -> Vec<Token> {
        if input.is_empty() {
            return Vec::new();
        }
        let mut tokens = Vec::new();
        let mut i = 0;
        while i < input.len() {
            let base_class = CHAR_CLASS_LUT[input[i] as usize];
            let mut run_len = 1;
            while i + run_len < input.len()
                && CHAR_CLASS_LUT[input[i + run_len] as usize] == base_class
            {
                run_len += 1;
            }
            match base_class {
                CharClass::Letter | CharClass::Digit => {
                    if base_class == CharClass::Letter
                        && run_len >= 1
                        && run_len <= MAX_SPECIAL_TOKEN_LEN
                    {
                        let mut upper = [0u8; MAX_RUN];
                        let buf_len = run_len.min(MAX_RUN);
                        for j in 0..buf_len {
                            upper[j] = TO_UPPER[input[i + j] as usize];
                        }
                        if let Some(special) = ref_special_token(&upper[..buf_len]) {
                            tokens.push(special);
                            i += run_len;
                            continue;
                        }
                    }
                    let r = (run_len - 1).min(MAX_RUN - 1);
                    let base = if base_class == CharClass::Letter {
                        Token::C1
                    } else {
                        Token::D1
                    };
                    tokens.push(unsafe {
                        std::mem::transmute::<u8, Token>(base as u8 + r as u8)
                    });
                }
                CharClass::Space => tokens.push(Token::Space),
                CharClass::Symbol(sym) => tokens.push(sym),
            }
            i += run_len;
        }
        tokens
    }

    fn ref_special_token(upper: &[u8]) -> Option<Token> {
        if upper.len() == 1 {
            return match upper[0] {
                b'T' => Some(Token::T),
                b'Z' => Some(Token::Zone),
                _ => None,
            };
        }
        let s = std::str::from_utf8(upper).ok()?;
        match s {
            "AM" | "PM" => Some(Token::Apm),
            "JAN" | "FEB" | "MAR" | "APR" | "MAY" | "JUN" | "JUL" | "AUG" | "SEP" | "OCT"
            | "NOV" | "DEC" => Some(Token::Month),
            "MON" | "TUE" | "WED" | "THU" | "FRI" | "SAT" | "SUN" => Some(Token::Day),
            "UTC" | "GMT" | "EST" | "EDT" | "CST" | "CDT" | "MST" | "MDT" | "PST" | "PDT"
            | "JST" | "KST" | "IST" | "MSK" | "CET" | "BST" | "HST" | "HDT" | "NST"
            | "NDT" => Some(Token::Zone),
            "CEST" | "NZST" | "NZDT" | "ACST" | "ACDT" | "AEST" | "AEDT" | "AWST" | "AWDT"
            | "AKST" | "AKDT" | "CHST" | "CHDT" => Some(Token::Zone),
            "WARN" | "WARNING" => Some(Token::Warn),
            "CRIT" | "CRITICAL" => Some(Token::Critical),
            "FATAL" => Some(Token::Fatal),
            "ERROR" => Some(Token::Error),
            "PANIC" => Some(Token::Panic),
            "ALERT" => Some(Token::Alert),
            "EMERG" | "EMERGENCY" => Some(Token::Emergency),
            "SEVERE" => Some(Token::Severe),
            "EXCEPTION" => Some(Token::Exception),
            "CRASH" | "CRASHED" => Some(Token::Crash),
            "FAILED" | "FAILURE" => Some(Token::Failure),
            "DEADLOCK" => Some(Token::Deadlock),
            "TIMEOUT" => Some(Token::Timeout),
            _ => None,
        }
    }

    proptest! {
        #[test]
        fn prop_parity(input in proptest::collection::vec(any::<u8>(), 0..2048)) {
            let tokenizer = Tokenizer::new(0);
            let (rust_tokens, _) = tokenizer.tokenize(&input);
            let ref_tokens = reference_tokenize(&input);
            prop_assert_eq!(
                tokens_to_string(&rust_tokens),
                tokens_to_string(&ref_tokens),
                "divergence on input {:?} (first 64 bytes)",
                &input[..input.len().min(64)]
            );
        }

        #[test]
        fn prop_determinism(input in proptest::collection::vec(any::<u8>(), 0..512)) {
            let tokenizer = Tokenizer::new(0);
            let (t1, i1) = tokenizer.tokenize(&input);
            let (t2, i2) = tokenizer.tokenize(&input);
            prop_assert_eq!(t1, t2);
            prop_assert_eq!(i1, i2);
        }

        #[test]
        fn prop_case_insensitive(input in proptest::collection::vec(any::<u8>(), 1..256)) {
            let tokenizer = Tokenizer::new(0);
            let (tokens_orig, _) = tokenizer.tokenize(&input);
            let flipped: Vec<u8> = input.iter().map(|&b| {
                if b.is_ascii_lowercase() { b.to_ascii_uppercase() }
                else if b.is_ascii_uppercase() { b.to_ascii_lowercase() }
                else { b }
            }).collect();
            let (tokens_flipped, _) = tokenizer.tokenize(&flipped);
            prop_assert_eq!(tokens_orig, tokens_flipped);
        }

        #[test]
        fn prop_digit_collapsing(input in proptest::collection::vec(any::<u8>(), 1..256)) {
            let tokenizer = Tokenizer::new(0);
            let (tokens_orig, _) = tokenizer.tokenize(&input);
            let swapped: Vec<u8> = input.iter().map(|&b| {
                if b.is_ascii_digit() { b'0' + (9 - (b - b'0')) } else { b }
            }).collect();
            let (tokens_swapped, _) = tokenizer.tokenize(&swapped);
            prop_assert_eq!(tokens_orig, tokens_swapped);
        }

        #[test]
        fn prop_truncation(
            input in proptest::collection::vec(any::<u8>(), 1..512),
            max_bytes in 1usize..512
        ) {
            let n = max_bytes.min(input.len());
            let tokenizer_limited = Tokenizer::new(n);
            let tokenizer_unlimited = Tokenizer::new(0);
            let (tokens_limited, indices_limited) = tokenizer_limited.tokenize(&input);
            let (tokens_prefix, indices_prefix) = tokenizer_unlimited.tokenize(&input[..n]);
            prop_assert_eq!(tokens_limited, tokens_prefix);
            prop_assert_eq!(indices_limited, indices_prefix);
        }
    }
}
