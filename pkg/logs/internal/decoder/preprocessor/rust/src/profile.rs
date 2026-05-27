// Profiling module — contains variant tokenizers to isolate bottlenecks.
// Not part of the public API.

use crate::keywords;
use crate::tokens::{CharClass, Token, CHAR_CLASS_LUT, MAX_RUN, MAX_SPECIAL_TOKEN_LEN, TO_UPPER};
use aho_corasick::AhoCorasick;

/// Variant 1: LUT-only scan, NO keyword matching at all.
/// Measures pure scan + run-length + allocation cost.
pub fn tokenize_lut_only(input: &[u8]) -> (Vec<Token>, Vec<i32>) {
    if input.is_empty() {
        return (Vec::new(), Vec::new());
    }
    let est = input.len() / 4 + 8;
    let mut tokens = Vec::with_capacity(est);
    let mut indices = Vec::with_capacity(est);
    let mut run: usize = 0;
    let mut last_class = CHAR_CLASS_LUT[input[0] as usize];

    for i in 1..input.len() {
        let current_class = CHAR_CLASS_LUT[input[i] as usize];
        if current_class != last_class {
            emit_no_keywords(&mut tokens, &mut indices, last_class, run, (i - 1) as i32);
            run = 0;
        } else {
            run += 1;
        }
        last_class = current_class;
    }
    emit_no_keywords(&mut tokens, &mut indices, last_class, run, (input.len() - 1) as i32);
    (tokens, indices)
}

#[inline]
fn emit_no_keywords(tokens: &mut Vec<Token>, indices: &mut Vec<i32>, class: CharClass, run: usize, idx: i32) {
    match class {
        CharClass::Letter => {
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::C1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Digit => {
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::D1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Space => { tokens.push(Token::Space); indices.push(idx - run as i32); }
        CharClass::Symbol(sym) => { tokens.push(sym); indices.push(idx - run as i32); }
    }
}

/// Variant 2: LUT scan + Go-style switch cascade for keywords (no AC).
/// Measures whether the switch approach is faster than AC.
pub fn tokenize_switch(input: &[u8]) -> (Vec<Token>, Vec<i32>) {
    if input.is_empty() {
        return (Vec::new(), Vec::new());
    }
    let est = input.len() / 4 + 8;
    let mut tokens = Vec::with_capacity(est);
    let mut indices = Vec::with_capacity(est);
    let mut run: usize = 0;
    let mut last_class = CHAR_CLASS_LUT[input[0] as usize];
    let mut str_buf = [0u8; MAX_RUN];
    let mut str_len: usize = 0;
    if last_class == CharClass::Letter {
        str_buf[0] = TO_UPPER[input[0] as usize];
        str_len = 1;
    }

    for i in 1..input.len() {
        let byte = input[i];
        let current_class = CHAR_CLASS_LUT[byte as usize];
        if current_class != last_class {
            emit_switch(&mut tokens, &mut indices, last_class, run, (i - 1) as i32, &str_buf, str_len);
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
    emit_switch(&mut tokens, &mut indices, last_class, run, (input.len() - 1) as i32, &str_buf, str_len);
    (tokens, indices)
}

#[inline]
fn emit_switch(
    tokens: &mut Vec<Token>,
    indices: &mut Vec<i32>,
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
                        b'T' => { tokens.push(Token::T); indices.push(idx); return; }
                        b'Z' => { tokens.push(Token::Zone); indices.push(idx); return; }
                        _ => {}
                    }
                } else if let Some(tok) = switch_keyword_lookup(str_buf, str_len) {
                    tokens.push(tok);
                    indices.push(idx - run as i32);
                    return;
                }
            }
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::C1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Digit => {
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::D1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Space => { tokens.push(Token::Space); indices.push(idx - run as i32); }
        CharClass::Symbol(sym) => { tokens.push(sym); indices.push(idx - run as i32); }
    }
}

#[inline]
fn switch_keyword_lookup(buf: &[u8; MAX_RUN], len: usize) -> Option<Token> {
    // Mirror of Go's getSpecialLongToken — length-dispatched switch
    match len {
        2 => {
            if (buf[0] == b'A' || buf[0] == b'P') && buf[1] == b'M' {
                return Some(Token::Apm);
            }
        }
        3 => {
            let b = [buf[0], buf[1], buf[2]];
            match &b {
                b"JAN" | b"FEB" | b"MAR" | b"APR" | b"MAY" | b"JUN" |
                b"JUL" | b"AUG" | b"SEP" | b"OCT" | b"NOV" | b"DEC" => return Some(Token::Month),
                b"MON" | b"TUE" | b"WED" | b"THU" | b"FRI" | b"SAT" | b"SUN" => return Some(Token::Day),
                b"UTC" | b"GMT" | b"EST" | b"EDT" | b"CST" | b"CDT" |
                b"MST" | b"MDT" | b"PST" | b"PDT" | b"JST" | b"KST" |
                b"IST" | b"MSK" | b"CET" | b"BST" | b"HST" | b"HDT" |
                b"NST" | b"NDT" => return Some(Token::Zone),
                _ => {}
            }
        }
        4 => {
            let b = [buf[0], buf[1], buf[2], buf[3]];
            match &b {
                b"WARN" => return Some(Token::Warn),
                b"CRIT" => return Some(Token::Critical),
                b"CEST" | b"NZST" | b"NZDT" | b"ACST" | b"ACDT" |
                b"AEST" | b"AEDT" | b"AWST" | b"AWDT" | b"AKST" |
                b"AKDT" | b"CHST" | b"CHDT" => return Some(Token::Zone),
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
            if &buf[..6] == b"SEVERE" { return Some(Token::Severe); }
            if &buf[..6] == b"FAILED" { return Some(Token::Failure); }
        }
        7 => {
            if &buf[..7] == b"WARNING" { return Some(Token::Warn); }
            if &buf[..7] == b"CRASHED" { return Some(Token::Crash); }
            if &buf[..7] == b"FAILURE" { return Some(Token::Failure); }
            if &buf[..7] == b"TIMEOUT" { return Some(Token::Timeout); }
        }
        8 => {
            if &buf[..8] == b"CRITICAL" { return Some(Token::Critical); }
            if &buf[..8] == b"DEADLOCK" { return Some(Token::Deadlock); }
        }
        9 => {
            if &buf[..9] == b"EMERGENCY" { return Some(Token::Emergency); }
            if &buf[..9] == b"EXCEPTION" { return Some(Token::Exception); }
        }
        _ => {}
    }
    None
}

/// Variant 3: LUT scan + AC on full input (bulk search), then merge matches.
/// Tests whether bulk AC is faster than per-emit AC.find().
pub fn tokenize_ac_bulk(ac: &AhoCorasick, input: &[u8]) -> (Vec<Token>, Vec<i32>) {
    if input.is_empty() {
        return (Vec::new(), Vec::new());
    }

    // Pre-scan: find all keyword matches and their byte ranges
    let mut keyword_map: Vec<Option<Token>> = vec![None; input.len()];
    let mut keyword_end: Vec<usize> = vec![0; input.len()];
    for m in ac.find_overlapping_iter(input) {
        keyword_map[m.start()] = Some(keywords::pattern_token(m.pattern().as_usize()));
        keyword_end[m.start()] = m.end();
    }

    let est = input.len() / 4 + 8;
    let mut tokens = Vec::with_capacity(est);
    let mut indices = Vec::with_capacity(est);
    let mut run: usize = 0;
    let mut last_class = CHAR_CLASS_LUT[input[0] as usize];
    let mut run_start: usize = 0;

    let mut str_buf = [0u8; MAX_RUN];
    let mut str_len: usize = 0;
    if last_class == CharClass::Letter {
        str_buf[0] = TO_UPPER[input[0] as usize];
        str_len = 1;
    }

    for i in 1..input.len() {
        let byte = input[i];
        let current_class = CHAR_CLASS_LUT[byte as usize];
        if current_class != last_class {
            emit_ac_bulk(&mut tokens, &mut indices, last_class, run, (i-1) as i32,
                         &str_buf, str_len, &keyword_map, &keyword_end, run_start);
            run = 0;
            str_len = 0;
            run_start = i;
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
    emit_ac_bulk(&mut tokens, &mut indices, last_class, run, (input.len()-1) as i32,
                 &str_buf, str_len, &keyword_map, &keyword_end, run_start);
    (tokens, indices)
}

#[inline]
fn emit_ac_bulk(
    tokens: &mut Vec<Token>,
    indices: &mut Vec<i32>,
    class: CharClass,
    run: usize,
    idx: i32,
    str_buf: &[u8; MAX_RUN],
    str_len: usize,
    keyword_map: &[Option<Token>],
    keyword_end: &[usize],
    run_start: usize,
) {
    let token_len = run + 1;
    match class {
        CharClass::Letter => {
            if token_len <= MAX_SPECIAL_TOKEN_LEN && str_len > 0 {
                if str_len == 1 {
                    match str_buf[0] {
                        b'T' => { tokens.push(Token::T); indices.push(idx); return; }
                        b'Z' => { tokens.push(Token::Zone); indices.push(idx); return; }
                        _ => {}
                    }
                } else if let Some(tok) = keyword_map[run_start] {
                    if keyword_end[run_start] == run_start + token_len {
                        tokens.push(tok);
                        indices.push(idx - run as i32);
                        return;
                    }
                }
            }
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::C1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Digit => {
            let r = run.min(MAX_RUN - 1);
            tokens.push(unsafe { std::mem::transmute::<u8, Token>(Token::D1 as u8 + r as u8) });
            indices.push(idx - run as i32);
        }
        CharClass::Space => { tokens.push(Token::Space); indices.push(idx - run as i32); }
        CharClass::Symbol(sym) => { tokens.push(sym); indices.push(idx - run as i32); }
    }
}
