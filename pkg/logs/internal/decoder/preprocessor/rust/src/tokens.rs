// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Token {
    Space = 0,

    // Special characters
    Colon = 1,
    Semicolon = 2,
    Dash = 3,
    Underscore = 4,
    Fslash = 5,
    Bslash = 6,
    Period = 7,
    Comma = 8,
    Singlequote = 9,
    Doublequote = 10,
    Backtick = 11,
    Tilda = 12,
    Star = 13,
    Plus = 14,
    Equal = 15,
    Parenopen = 16,
    Parenclose = 17,
    Braceopen = 18,
    Braceclose = 19,
    Bracketopen = 20,
    Bracketclose = 21,
    Ampersand = 22,
    Exclamation = 23,
    At = 24,
    Pound = 25,
    Dollar = 26,
    Percent = 27,
    Uparrow = 28,

    // Digit runs
    D1 = 29,
    D2 = 30,
    D3 = 31,
    D4 = 32,
    D5 = 33,
    D6 = 34,
    D7 = 35,
    D8 = 36,
    D9 = 37,
    D10 = 38,

    // Char runs
    C1 = 39,
    C2 = 40,
    C3 = 41,
    C4 = 42,
    C5 = 43,
    C6 = 44,
    C7 = 45,
    C8 = 46,
    C9 = 47,
    C10 = 48,

    // Calendar tokens
    Month = 49,
    Day = 50,
    Apm = 51,
    Zone = 52,
    T = 53,

    // Severity keywords
    Warn = 54,
    Fatal = 55,
    Error = 56,
    Panic = 57,
    Alert = 58,
    Severe = 59,
    Critical = 60,
    Emergency = 61,
    Exception = 62,
    Crash = 63,
    Failure = 64,
    Deadlock = 65,
    Timeout = 66,

    End = 67,
}

const _: () = assert!(Token::End as u8 == 67);
const _: () = assert!(Token::D1 as u8 == 29);
const _: () = assert!(Token::D10 as u8 == 38);
const _: () = assert!(Token::C1 as u8 == 39);
const _: () = assert!(Token::C10 as u8 == 48);
const _: () = assert!(Token::Month as u8 == 49);
const _: () = assert!(Token::Warn as u8 == 54);

pub const MAX_RUN: usize = 10;
pub const MAX_SPECIAL_TOKEN_LEN: usize = 9;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum CharClass {
    Letter,
    Digit,
    Space,
    Symbol(Token),
}

pub(crate) static CHAR_CLASS_LUT: [CharClass; 256] = make_char_class_lut();
pub(crate) static TO_UPPER: [u8; 256] = make_to_upper_lut();

const fn make_char_class_lut() -> [CharClass; 256] {
    let mut lut = [CharClass::Letter; 256]; // default: letter (includes non-ASCII)

    // Digits
    let mut c = b'0';
    while c <= b'9' {
        lut[c as usize] = CharClass::Digit;
        c += 1;
    }

    // Whitespace
    lut[b' ' as usize] = CharClass::Space;
    lut[b'\t' as usize] = CharClass::Space;
    lut[b'\n' as usize] = CharClass::Space;
    lut[b'\r' as usize] = CharClass::Space;

    // Punctuation — each maps to its specific Token
    lut[b':' as usize] = CharClass::Symbol(Token::Colon);
    lut[b';' as usize] = CharClass::Symbol(Token::Semicolon);
    lut[b'-' as usize] = CharClass::Symbol(Token::Dash);
    lut[b'_' as usize] = CharClass::Symbol(Token::Underscore);
    lut[b'/' as usize] = CharClass::Symbol(Token::Fslash);
    lut[b'\\' as usize] = CharClass::Symbol(Token::Bslash);
    lut[b'.' as usize] = CharClass::Symbol(Token::Period);
    lut[b',' as usize] = CharClass::Symbol(Token::Comma);
    lut[b'\'' as usize] = CharClass::Symbol(Token::Singlequote);
    lut[b'"' as usize] = CharClass::Symbol(Token::Doublequote);
    lut[b'`' as usize] = CharClass::Symbol(Token::Backtick);
    lut[b'~' as usize] = CharClass::Symbol(Token::Tilda);
    lut[b'*' as usize] = CharClass::Symbol(Token::Star);
    lut[b'+' as usize] = CharClass::Symbol(Token::Plus);
    lut[b'=' as usize] = CharClass::Symbol(Token::Equal);
    lut[b'(' as usize] = CharClass::Symbol(Token::Parenopen);
    lut[b')' as usize] = CharClass::Symbol(Token::Parenclose);
    lut[b'{' as usize] = CharClass::Symbol(Token::Braceopen);
    lut[b'}' as usize] = CharClass::Symbol(Token::Braceclose);
    lut[b'[' as usize] = CharClass::Symbol(Token::Bracketopen);
    lut[b']' as usize] = CharClass::Symbol(Token::Bracketclose);
    lut[b'&' as usize] = CharClass::Symbol(Token::Ampersand);
    lut[b'!' as usize] = CharClass::Symbol(Token::Exclamation);
    lut[b'@' as usize] = CharClass::Symbol(Token::At);
    lut[b'#' as usize] = CharClass::Symbol(Token::Pound);
    lut[b'$' as usize] = CharClass::Symbol(Token::Dollar);
    lut[b'%' as usize] = CharClass::Symbol(Token::Percent);
    lut[b'^' as usize] = CharClass::Symbol(Token::Uparrow);

    lut
}

const fn make_to_upper_lut() -> [u8; 256] {
    let mut lut = [0u8; 256];
    let mut i = 0usize;
    while i < 256 {
        lut[i] = i as u8;
        i += 1;
    }
    let mut c = b'a';
    while c <= b'z' {
        lut[c as usize] = c - 32;
        c += 1;
    }
    lut
}

pub fn token_to_string(token: Token) -> &'static str {
    match token {
        Token::Space => " ",
        Token::Colon => ":",
        Token::Semicolon => ";",
        Token::Dash => "-",
        Token::Underscore => "_",
        Token::Fslash => "/",
        Token::Bslash => "\\",
        Token::Period => ".",
        Token::Comma => ",",
        Token::Singlequote => "'",
        Token::Doublequote => "\"",
        Token::Backtick => "`",
        Token::Tilda => "~",
        Token::Star => "*",
        Token::Plus => "+",
        Token::Equal => "=",
        Token::Parenopen => "(",
        Token::Parenclose => ")",
        Token::Braceopen => "{",
        Token::Braceclose => "}",
        Token::Bracketopen => "[",
        Token::Bracketclose => "]",
        Token::Ampersand => "&",
        Token::Exclamation => "!",
        Token::At => "@",
        Token::Pound => "#",
        Token::Dollar => "$",
        Token::Percent => "%",
        Token::Uparrow => "^",
        Token::D1 => "D",
        Token::D2 => "DD",
        Token::D3 => "DDD",
        Token::D4 => "DDDD",
        Token::D5 => "DDDDD",
        Token::D6 => "DDDDDD",
        Token::D7 => "DDDDDDD",
        Token::D8 => "DDDDDDDD",
        Token::D9 => "DDDDDDDDD",
        Token::D10 => "DDDDDDDDDD",
        Token::C1 => "C",
        Token::C2 => "CC",
        Token::C3 => "CCC",
        Token::C4 => "CCCC",
        Token::C5 => "CCCCC",
        Token::C6 => "CCCCCC",
        Token::C7 => "CCCCCCC",
        Token::C8 => "CCCCCCCC",
        Token::C9 => "CCCCCCCCC",
        Token::C10 => "CCCCCCCCCC",
        Token::Month => "MTH",
        Token::Day => "DAY",
        Token::Apm => "PM",
        Token::Zone => "ZONE",
        Token::T => "T",
        Token::Warn => "WARN",
        Token::Fatal => "FATAL",
        Token::Error => "ERROR",
        Token::Panic => "PANIC",
        Token::Alert => "ALERT",
        Token::Severe => "SEVERE",
        Token::Critical => "CRIT",
        Token::Emergency => "EMERG",
        Token::Exception => "EXCEPTION",
        Token::Crash => "CRASH",
        Token::Failure => "FAILURE",
        Token::Deadlock => "DEADLOCK",
        Token::Timeout => "TIMEOUT",
        Token::End => "",
    }
}

pub fn tokens_to_string(tokens: &[Token]) -> String {
    let mut s = String::new();
    for &t in tokens {
        s.push_str(token_to_string(t));
    }
    s
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_char_class_letters() {
        for c in b'a'..=b'z' {
            assert_eq!(CHAR_CLASS_LUT[c as usize], CharClass::Letter);
        }
        for c in b'A'..=b'Z' {
            assert_eq!(CHAR_CLASS_LUT[c as usize], CharClass::Letter);
        }
        // Non-ASCII bytes are letters
        for c in 128u8..=255 {
            assert_eq!(CHAR_CLASS_LUT[c as usize], CharClass::Letter);
        }
    }

    #[test]
    fn test_char_class_digits() {
        for c in b'0'..=b'9' {
            assert_eq!(CHAR_CLASS_LUT[c as usize], CharClass::Digit);
        }
    }

    #[test]
    fn test_char_class_space() {
        assert_eq!(CHAR_CLASS_LUT[b' ' as usize], CharClass::Space);
        assert_eq!(CHAR_CLASS_LUT[b'\t' as usize], CharClass::Space);
        assert_eq!(CHAR_CLASS_LUT[b'\n' as usize], CharClass::Space);
        assert_eq!(CHAR_CLASS_LUT[b'\r' as usize], CharClass::Space);
    }

    #[test]
    fn test_char_class_symbols() {
        assert_eq!(CHAR_CLASS_LUT[b':' as usize], CharClass::Symbol(Token::Colon));
        assert_eq!(CHAR_CLASS_LUT[b'-' as usize], CharClass::Symbol(Token::Dash));
        assert_eq!(CHAR_CLASS_LUT[b'!' as usize], CharClass::Symbol(Token::Exclamation));
        assert_eq!(CHAR_CLASS_LUT[b'@' as usize], CharClass::Symbol(Token::At));
    }

    #[test]
    fn test_to_upper() {
        for c in b'a'..=b'z' {
            assert_eq!(TO_UPPER[c as usize], c - 32);
        }
        for c in b'A'..=b'Z' {
            assert_eq!(TO_UPPER[c as usize], c);
        }
        for c in b'0'..=b'9' {
            assert_eq!(TO_UPPER[c as usize], c);
        }
    }

    #[test]
    fn test_tokens_to_string() {
        assert_eq!(tokens_to_string(&[Token::C3, Token::D3]), "CCCDDD");
        assert_eq!(tokens_to_string(&[Token::Fatal]), "FATAL");
        assert_eq!(tokens_to_string(&[Token::Space]), " ");
        assert_eq!(tokens_to_string(&[]), "");
    }
}
