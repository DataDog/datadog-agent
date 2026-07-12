// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package syslog parses RFC 5424 and RFC 3164 (BSD) syslog messages.
//
// Two public entry points:
//
//   - Parse(line) for network input (with PRI, auto-detects RFC 5424 vs BSD)
//   - ParseBSDLine(line) for on-disk log files (no PRI, e.g. /var/log/syslog)
//
// Best-effort: on malformed input the parser extracts as many fields as
// possible and returns a partial SyslogMessage (Partial=true) alongside the error.
// Fatal errors (empty input, no PRI) return Pri=-1 and the raw content in Msg.
//
// The parser is allocation-light: it returns SyslogMessage by value (no heap
// escape) and string fields are standard Go strings (independent copies of the
// input). The Msg field ([]byte) aliases the input buffer for zero-copy
// performance; callers that need to retain Msg after reusing the input buffer
// must copy it. All other (string) fields are safe to hold indefinitely.
package syslog

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Pre-allocated sentinel errors — avoids fmt.Errorf allocations on error paths.
var (
	errEmpty          = errors.New("empty message")
	errNoPRIClose     = errors.New("invalid PRI: no '>' found")
	errMissingSD      = errors.New("missing STRUCTURED-DATA")
	errHeaderTooShort = errors.New("header too short: need 5 SPs after VERSION")

	errPRIFormat   = errors.New("invalid PRI: must be <1-3 digits>")
	errPRINonDigit = errors.New("invalid PRI: non-digit in PRIVAL")
	errPRIRange    = errors.New("invalid PRI: PRIVAL > 191")

	errVersionEmpty = errors.New("invalid VERSION: empty")
	errVersionStart = errors.New("invalid VERSION: must start with nonzero digit")
	errVersionLen   = errors.New("invalid VERSION: max 3 digits")
	errVersionDigit = errors.New("invalid VERSION: non-digit")

	errTruncatedAfterPRI = errors.New("truncated message: nothing after PRI")
	errNoSPAfterVersion  = errors.New("header too short: no SP after VERSION")

	errSDEmpty        = errors.New("structured data: empty")
	errSDExpected     = errors.New("structured data: expected '-' or '['")
	errSDElemOpen     = errors.New("SD-ELEMENT: expected '['")
	errSDIDInvalid    = errors.New("SD-ELEMENT: invalid character in SD-ID")
	errSDIDTooLong    = errors.New("SD-ELEMENT: SD-ID too long")
	errSDIDRequired   = errors.New("SD-ELEMENT: SD-ID required after '['")
	errSDElemExpect   = errors.New("SD-ELEMENT: expected SP or ']'")
	errSDElemUnclosed = errors.New("SD-ELEMENT: unclosed '['")

	errSDParamEmpty    = errors.New("SD-PARAM: empty")
	errSDParamNameBad  = errors.New("SD-PARAM: invalid in PARAM-NAME")
	errSDParamNameLong = errors.New("SD-PARAM: PARAM-NAME too long")
	errSDParamNoEq     = errors.New("SD-PARAM: expected '=' after PARAM-NAME")
	errSDParamNoQuote  = errors.New("SD-PARAM: expected '\"' after '='")
	errSDParamTrailBS  = errors.New("SD-PARAM: backslash at end of value")
	errSDParamUnclosed = errors.New("SD-PARAM: unclosed '\"'")

	// BSD-specific errors
	errBSDTimestamp = errors.New("BSD: invalid timestamp format")
	errBSDHostname  = errors.New("BSD: missing hostname")
)

const nilvalue = "-"

// RFC 5424 §6.3: PRINTUSASCII range and max name length for SD-ID / PARAM-NAME.
const (
	printUSASCIIMin = 33  // '!' — lower bound of PRINTUSASCII (%d33-126)
	printUSASCIIMax = 126 // '~' — upper bound of PRINTUSASCII
	maxSDNameLen    = 32  // max length for SD-ID and PARAM-NAME
)

// SyslogMessage is a parsed syslog message (RFC 5424 or RFC 3164/BSD).
//
// String fields are independent copies of the input. The Msg field aliases
// the input buffer; callers must copy it before reusing the input.
type SyslogMessage struct {
	Pri       int    // PRIVAL 0-191 (Facility*8 + Severity); -1 if absent (file input)
	Version   string // VERSION (e.g. "1") for RFC 5424; "" for BSD
	Timestamp string // TIMESTAMP or "-"
	Hostname  string // HOSTNAME or "-"
	AppName   string // APP-NAME or "-"
	ProcID    string // PROCID or "-"
	MsgID     string // MSGID or "-"

	// StructuredData is the parsed STRUCTURED-DATA as a nested map.
	// Outer key is the SD-ID, inner map is PARAM-NAME to PARAM-VALUE.
	// nil for BSD messages and NILVALUE ("-").
	StructuredData map[string]map[string]string

	// Msg is the message body (MSG for RFC 5424, CONTENT for BSD)
	// after stripping optional UTF-8 BOM (RFC 5424 only).
	Msg []byte

	// Partial is true when the parser recovered from malformed input.
	// Some fields may be populated while others remain at defaults.
	Partial bool
}

// toHeaderString converts a header field byte slice to a string.
// Returns the nilvalue constant for "-" to avoid allocation.
func toHeaderString(b []byte) string {
	if len(b) == 1 && b[0] == '-' {
		return nilvalue
	}
	return string(b)
}

// Parse parses a single syslog message from network input (TCP or UDP).
// The message MUST have a PRI header (<digits>). After parsing PRI, the
// format is auto-detected:
//
//   - Digit after '>' → RFC 5424
//   - Letter after '>' → BSD (RFC 3164)
//
// Returns SyslogMessage by value for zero heap allocation on the happy path.
// On malformed input, returns a partial SyslogMessage (Partial=true) with as many
// fields populated as possible, alongside a non-nil error.
func Parse(line []byte) (SyslogMessage, error) {
	if len(line) == 0 {
		return SyslogMessage{Pri: -1}, errEmpty
	}

	// --- Extract PRI (find '>') ---
	// PRI is at most 5 chars: '<' + 3 digits + '>'. Cap scan at 5.
	gtPos := -1
	limit := len(line)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		if line[i] == '>' {
			gtPos = i
			break
		}
	}
	if gtPos < 0 {
		return SyslogMessage{Pri: -1, Msg: line, Partial: true}, errNoPRIClose
	}

	// --- Validate PRI ---
	if gtPos < 2 || line[0] != '<' {
		return SyslogMessage{Pri: -1, Msg: line, Partial: true}, errPRIFormat
	}
	pri := 0
	for i := 1; i < gtPos; i++ {
		d := line[i]
		if d < '0' || d > '9' {
			return SyslogMessage{Pri: -1, Msg: line, Partial: true}, errPRINonDigit
		}
		pri = pri*10 + int(d-'0')
	}

	// Best-effort: accept PRI > 191 but remember the error.
	var priErr error
	if pri > 191 {
		priErr = errPRIRange
	}

	// --- Dispatch based on byte after '>' ---
	pos := gtPos + 1
	if pos >= len(line) {
		return SyslogMessage{Pri: pri, Msg: line, Partial: true}, errTruncatedAfterPRI
	}

	var msg SyslogMessage
	var err error

	b := line[pos]
	switch {
	case b >= '0' && b <= '9':
		// A digit after PRI *may* be an RFC 5424 VERSION, but many network
		// appliances (PAN-OS, Cisco) emit no-timestamp BSD messages whose
		// CONTENT starts with a digit (e.g. "<134>1,2026/06/23,...,TRAFFIC,...").
		// Only dispatch to the RFC 5424 parser when the bytes actually form a
		// VERSION token (1-3 digits) followed by SP; otherwise treat the
		// remainder as MSG CONTENT per RFC 3164 §4.3.2.
		if isRFC5424Header(line, pos) {
			msg, err = parseRFC5424(line, pri, pos)
		} else {
			msg = parseBSDNoTimestamp(line, pri, pos)
		}
	case (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z'):
		msg, err = parseBSD(line, pri, pos)
	default:
		// RFC 3164 §4.3.2: valid PRI, but what follows is neither a digit
		// (RFC 5424 VERSION) nor a letter (BSD TIMESTAMP month). Treat the
		// remainder as MSG CONTENT with no TIMESTAMP, HOSTNAME, or TAG.
		msg = parseBSDNoTimestamp(line, pri, pos)
	}

	// Apply PRI range warning — join with any downstream error so neither is lost.
	if priErr != nil {
		msg.Partial = true
		err = errors.Join(priErr, err)
	}

	return msg, err
}

// ParseBSDLine parses a single BSD syslog line from an on-disk log file
// (e.g. /var/log/syslog). The message has no PRI header; Pri is set to -1.
//
// Returns SyslogMessage by value. On malformed input, returns a partial SyslogMessage
// (Partial=true) alongside a non-nil error.
func ParseBSDLine(line []byte) (SyslogMessage, error) {
	if len(line) == 0 {
		return SyslogMessage{Pri: -1}, errEmpty
	}
	return parseBSD(line, -1, 0)
}

// ---------------------------------------------------------------------------
// RFC 5424 parsing
// ---------------------------------------------------------------------------

// isRFC5424Header reports whether the bytes starting at line[pos] look like an
// RFC 5424 VERSION field immediately followed by SP. The VERSION token is a run
// of digits; a genuine RFC 5424 line is "VERSION SP TIMESTAMP SP ...". We only
// require that the leading digit run is terminated by a space here — the strict
// VERSION validation (nonzero first digit, max 3 digits) is left to
// parseRFC5424, which reports a precise error for malformed-but-5424-shaped
// lines (e.g. "<14>0 ..." or "<14>1234 ...").
//
// This distinguishes those from no-timestamp BSD content that merely begins
// with a digit but is NOT followed by a space ("<134>1,2026/06/23,...", the
// PAN-OS/Cisco CSV dialect), which must be handled as RFC 3164 §4.3.2 MSG
// CONTENT instead of being forced through the RFC 5424 parser.
func isRFC5424Header(line []byte, pos int) bool {
	i := pos
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	// The digit run must be immediately followed by SP. A non-digit delimiter
	// (',', '/', ...) or end-of-line means this is not an RFC 5424 header.
	return i > pos && i < len(line) && line[i] == ' '
}

// parseRFC5424 parses an RFC 5424 message starting at line[pos] (the VERSION
// field). PRI has already been extracted.
//
// Best-effort: on incomplete headers or malformed SD, populates as many fields
// as possible, sets Partial=true, and returns the error.
func parseRFC5424(line []byte, pri int, pos int) (SyslogMessage, error) {
	// --- VERSION: from pos to first SP ---
	sp := pos
	for sp < len(line) && line[sp] != ' ' {
		sp++
	}
	if sp >= len(line) {
		return SyslogMessage{Pri: pri, Msg: line[pos:], Partial: true}, errNoSPAfterVersion
	}
	verRaw := line[pos:sp]

	// Validate VERSION: NONZERO-DIGIT 0*2DIGIT
	if len(verRaw) == 0 {
		return SyslogMessage{Pri: pri, Msg: line[pos:], Partial: true}, errVersionEmpty
	}
	if verRaw[0] < '1' || verRaw[0] > '9' {
		return SyslogMessage{Pri: pri, Msg: line[pos:], Partial: true}, errVersionStart
	}
	if len(verRaw) > 3 {
		return SyslogMessage{Pri: pri, Msg: line[pos:], Partial: true}, errVersionLen
	}
	for i := 1; i < len(verRaw); i++ {
		if verRaw[i] < '0' || verRaw[i] > '9' {
			return SyslogMessage{Pri: pri, Msg: line[pos:], Partial: true}, errVersionDigit
		}
	}

	// --- Find SPs after VERSION to delimit header fields ---
	// Fields: TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP ...
	//
	// NOTE: This uses a simple space-counting heuristic. It works because
	// RFC 5424 HEADER fields are single tokens (no embedded spaces), but it
	// will mis-parse non-conformant TIMESTAMP values that contain extra spaces.
	// This is an intentional performance trade-off vs. regex or field-by-field parsing.
	var spPos [5]int
	found := 0
	for i := sp + 1; i < len(line); i++ {
		if line[i] == ' ' {
			spPos[found] = i
			found++
			if found == 5 {
				break
			}
		}
	}

	// Build message with defaults for all fields.
	msg := SyslogMessage{
		Pri:       pri,
		Version:   toHeaderString(verRaw),
		Timestamp: nilvalue,
		Hostname:  nilvalue,
		AppName:   nilvalue,
		ProcID:    nilvalue,
		MsgID:     nilvalue,
	}

	// --- Best-effort: extract available header fields in order ---

	if found < 1 {
		// Only VERSION; remainder is MSG.
		if sp+1 < len(line) {
			msg.Msg = line[sp+1:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}
	msg.Timestamp = toHeaderString(line[sp+1 : spPos[0]])

	if found < 2 {
		if spPos[0]+1 < len(line) {
			msg.Msg = line[spPos[0]+1:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}
	msg.Hostname = toHeaderString(line[spPos[0]+1 : spPos[1]])

	if found < 3 {
		if spPos[1]+1 < len(line) {
			msg.Msg = line[spPos[1]+1:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}
	msg.AppName = toHeaderString(line[spPos[1]+1 : spPos[2]])

	if found < 4 {
		if spPos[2]+1 < len(line) {
			msg.Msg = line[spPos[2]+1:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}
	msg.ProcID = toHeaderString(line[spPos[2]+1 : spPos[3]])

	if found < 5 {
		if spPos[3]+1 < len(line) {
			msg.Msg = line[spPos[3]+1:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}
	msg.MsgID = toHeaderString(line[spPos[3]+1 : spPos[4]])

	// --- Parse STRUCTURED-DATA ---
	rest := line[spPos[4]+1:]
	if len(rest) == 0 {
		msg.Partial = true
		return msg, errMissingSD
	}

	sd, sdLen, sdErr := parseStructuredData(rest)
	if sdErr != nil {
		msg.StructuredData = sd // preserve any successfully-parsed SD elements
		msg.Msg = rest[sdLen:]  // remainder after last successful parse
		msg.Partial = true
		return msg, sdErr
	}
	msg.StructuredData = sd

	// --- Optional [SP MSG] ---
	if sdLen < len(rest) && rest[sdLen] == ' ' {
		rawMsg := rest[sdLen+1:]
		msg.Msg = stripBOM(rawMsg)
	}

	return msg, nil
}

// ---------------------------------------------------------------------------
// BSD (RFC 3164) parsing
// ---------------------------------------------------------------------------

// parseBSDNoTimestamp implements RFC 3164 Section 4.3.2: a message with a valid
// PRI but no valid TIMESTAMP. Per the RFC, the receiver treats the remainder
// after PRI as the CONTENT field of the MSG. TAG "cannot be determined and will
// not be included." TIMESTAMP and HOSTNAME are left as nilvalue for the caller
// to fill from receiver-local context (current time, sender address).
func parseBSDNoTimestamp(line []byte, pri int, pos int) SyslogMessage {
	return SyslogMessage{
		Pri:       pri,
		Timestamp: nilvalue,
		Hostname:  nilvalue,
		AppName:   nilvalue,
		ProcID:    nilvalue,
		MsgID:     nilvalue,
		Msg:       line[pos:],
	}
}

// parseBSD parses a BSD syslog message starting at line[pos]. PRI has already
// been extracted (pri=-1 for file input where PRI is absent).
//
// Best-effort: extracts TIMESTAMP, HOSTNAME, TAG (APP-NAME + PROCID), and
// CONTENT (MSG) in order. On failure at any step, remaining fields get
// defaults and unparsed data goes into Msg.
func parseBSD(line []byte, pri int, pos int) (SyslogMessage, error) {
	msg := SyslogMessage{
		Pri:       pri,
		Timestamp: nilvalue,
		Hostname:  nilvalue,
		AppName:   nilvalue,
		ProcID:    nilvalue,
		MsgID:     nilvalue,
	}

	// --- TIMESTAMP ---
	// Try standard 15-byte BSD format first: "Mmm dd hh:mm:ss"
	// Then try 20-byte variant with year: "Mmm DD YYYY HH:MM:SS"
	// (used by some network appliances that deviate from RFC 3164).
	tsLen := 0
	if pos+15 <= len(line) && isValidBSDTimestamp(line[pos:pos+15]) {
		tsLen = 15
	} else if pos+20 <= len(line) && isValidBSDTimestampWithYear(line[pos:pos+20]) {
		tsLen = 20
	}

	if tsLen == 0 {
		if pos < len(line) && pri >= 0 {
			// RFC 3164 §4.3.2: valid PRI, content present but no valid
			// timestamp. Treat remainder as MSG CONTENT.
			return parseBSDNoTimestamp(line, pri, pos), nil
		}
		if pos < len(line) {
			msg.Msg = line[pos:]
		}
		msg.Partial = true
		return msg, errBSDTimestamp
	}
	msg.Timestamp = string(line[pos : pos+tsLen])
	pos += tsLen

	// --- SP + HOSTNAME ---
	if pos >= len(line) || line[pos] != ' ' {
		msg.Partial = true
		return msg, errBSDHostname
	}
	pos++ // skip SP

	hostEnd := pos
	for hostEnd < len(line) && line[hostEnd] != ' ' {
		hostEnd++
	}
	if hostEnd == pos {
		msg.Partial = true
		return msg, errBSDHostname
	}
	msg.Hostname = string(line[pos:hostEnd])

	if hostEnd >= len(line) {
		return msg, nil
	}
	pos = hostEnd + 1 // skip SP after hostname

	if pos >= len(line) {
		return msg, nil
	}

	// --- TAG + CONTENT ---
	rest := line[pos:]

	// Detect "double-header" formats where an ISO 8601 timestamp appears in
	// the TAG position (e.g. Cisco FTD: "YYYY-MM-DDThh:mm:ssZ hostname ...").
	// No real TAG is present; treat the entire remainder as MSG.
	if looksLikeISOTimestamp(rest) {
		msg.Msg = rest
		return msg, nil
	}

	parseBSDTag(&msg, rest)
	return msg, nil
}

// parseBSDTag extracts APP-NAME, PROCID, and MSG from the TAG+CONTENT portion
// of a BSD syslog message. It modifies msg in place.
//
// TAG patterns:
//
//	appname[pid]: content   → AppName=appname, ProcID=pid, Msg=content
//	appname: content        → AppName=appname, ProcID="-", Msg=content
//	appname content         → AppName=appname, ProcID="-", Msg=content
//	-- MARK --              → AppName="-", Msg="-- MARK --" (non-alpha start)
//
// NOTE: RFC 3164 limits TAG to 32 alphanumeric characters, but real-world
// emitters regularly exceed this. We intentionally accept unbounded TAG lengths
// for compatibility with common syslog implementations (rsyslog, syslog-ng).
func parseBSDTag(msg *SyslogMessage, rest []byte) {
	if len(rest) == 0 {
		return
	}

	// TAG must start with an alphanumeric character. If not, the entire
	// rest is CONTENT (e.g. "-- MARK --").
	first := rest[0]
	if !isAlphaNumeric(first) {
		msg.Msg = rest
		return
	}

	// Scan for the first TAG delimiter: '[', ':', or ' '.
	delimIdx := -1
	delimChar := byte(0)
	for i := 0; i < len(rest); i++ {
		if rest[i] == '[' || rest[i] == ':' || rest[i] == ' ' {
			delimIdx = i
			delimChar = rest[i]
			break
		}
	}

	if delimIdx < 0 {
		// No delimiter — entire rest is APP-NAME with no MSG.
		candidate := string(rest)
		if !isPlausibleAppName(candidate) {
			msg.Msg = rest
			return
		}
		msg.AppName = candidate
		return
	}

	// Validate the candidate TAG before committing. Many network appliances
	// omit a TAG entirely, causing the parser to latch onto the first data
	// token (a CSV field, a year, etc.). When the candidate fails the
	// plausibility check, treat the full remainder as MSG with no TAG.
	candidate := string(rest[:delimIdx])
	if !isPlausibleAppName(candidate) {
		msg.Msg = rest
		return
	}

	switch delimChar {
	case '[':
		// appname[pid]: content
		msg.AppName = candidate

		// Find closing ']'.
		closePos := delimIdx + 1
		for closePos < len(rest) && rest[closePos] != ']' {
			closePos++
		}
		if closePos >= len(rest) {
			// Unclosed bracket — best effort: rest after APP-NAME is MSG.
			msg.Msg = rest[delimIdx:]
			msg.Partial = true
			return
		}

		msg.ProcID = string(rest[delimIdx+1 : closePos])

		// Consume optional ':' and SP after ']'.
		contentStart := closePos + 1
		if contentStart < len(rest) && rest[contentStart] == ':' {
			contentStart++
		}
		if contentStart < len(rest) && rest[contentStart] == ' ' {
			contentStart++
		}
		if contentStart < len(rest) {
			msg.Msg = rest[contentStart:]
		}

	case ':':
		// Before committing TAG, check if rest is actually the start of a
		// CEF or LEEF header (e.g. "CEF:0|Vendor|..." or "LEEF:1.0|...").
		// Per the CEF spec, the syslog prefix is just "Mmm dd HH:MM:SS host"
		// and the body starts immediately with "CEF:Version|...". There is no
		// syslog TAG. If we consumed "CEF:" as TAG, the MSG would lose its
		// prefix and CEF detection downstream would fail.
		if isCEFLEEFStart(rest, delimIdx) {
			msg.Msg = rest
			return
		}

		// appname: content
		msg.AppName = candidate

		contentStart := delimIdx + 1
		if contentStart < len(rest) && rest[contentStart] == ' ' {
			contentStart++
		}
		if contentStart < len(rest) {
			msg.Msg = rest[contentStart:]
		}

	case ' ':
		// appname content (no colon or bracket delimiter)
		msg.AppName = candidate

		contentStart := delimIdx + 1
		if contentStart < len(rest) {
			msg.Msg = rest[contentStart:]
		}
	}
}

// isCEFLEEFStart returns true when the TAG candidate + delimiter is actually
// the opening of a CEF or LEEF header. The CEF spec defines the syslog format
// as "<syslog prefix> CEF:Version|…" with no syslog TAG — the "CEF:" is part
// of the body. We detect this by checking that the candidate equals "CEF" or
// "LEEF" (case-sensitive per spec), the delimiter is ':', and the byte
// immediately after ':' is a digit (the version number).
func isCEFLEEFStart(rest []byte, colonIdx int) bool {
	candidate := rest[:colonIdx]
	if len(candidate) != 3 && len(candidate) != 4 {
		return false
	}
	isCEF := len(candidate) == 3 && candidate[0] == 'C' && candidate[1] == 'E' && candidate[2] == 'F'
	isLEEF := len(candidate) == 4 && candidate[0] == 'L' && candidate[1] == 'E' && candidate[2] == 'E' && candidate[3] == 'F'
	if !isCEF && !isLEEF {
		return false
	}
	afterColon := colonIdx + 1
	return afterColon < len(rest) && rest[afterColon] >= '0' && rest[afterColon] <= '9'
}

// isValidBSDTimestamp checks the 15-byte BSD timestamp "Mmm dd hh:mm:ss" for
// structural validity: valid month abbreviation plus spaces at [3] and [6],
// colons at [9] and [12]. This prevents false-positive matches on lines that
// coincidentally start with a month abbreviation (e.g. "December sales...").
func isValidBSDTimestamp(b []byte) bool {
	if len(b) < 15 {
		return false
	}
	if b[3] != ' ' || b[6] != ' ' || b[9] != ':' || b[12] != ':' {
		return false
	}
	return isValidMonthAbbrev(b)
}

// isValidBSDTimestampWithYear checks for the 20-byte variant "Mmm DD YYYY HH:MM:SS"
// used by some network appliances. This is a well-documented deviation from
// RFC 3164 that inserts a 4-digit year between the day and time fields:
//
//	Standard:  "May  4 21:09:42"      (15 bytes)
//	With year: "May 04 2026 21:09:42" (20 bytes)
//
// Structural checks: valid month at [0:3], spaces at [3], [6], [11],
// colons at [14] and [17], and digits for the year at [7:11].
func isValidBSDTimestampWithYear(b []byte) bool {
	if len(b) < 20 {
		return false
	}
	if b[3] != ' ' || b[6] != ' ' || b[11] != ' ' || b[14] != ':' || b[17] != ':' {
		return false
	}
	for _, i := range []int{7, 8, 9, 10} {
		if b[i] < '0' || b[i] > '9' {
			return false
		}
	}
	return isValidMonthAbbrev(b)
}

// isValidMonthAbbrev returns true if b[0:3] is a valid three-letter English
// month abbreviation (Jan, Feb, Mar, ..., Dec).
func isValidMonthAbbrev(b []byte) bool {
	switch b[0] {
	case 'J':
		return (b[1] == 'a' && b[2] == 'n') || // Jan
			(b[1] == 'u' && (b[2] == 'n' || b[2] == 'l')) // Jun, Jul
	case 'F':
		return b[1] == 'e' && b[2] == 'b' // Feb
	case 'M':
		return b[1] == 'a' && (b[2] == 'r' || b[2] == 'y') // Mar, May
	case 'A':
		return (b[1] == 'p' && b[2] == 'r') || // Apr
			(b[1] == 'u' && b[2] == 'g') // Aug
	case 'S':
		return b[1] == 'e' && b[2] == 'p' // Sep
	case 'O':
		return b[1] == 'c' && b[2] == 't' // Oct
	case 'N':
		return b[1] == 'o' && b[2] == 'v' // Nov
	case 'D':
		return b[1] == 'e' && b[2] == 'c' // Dec
	}
	return false
}

// isAlphaNumeric returns true for ASCII letters and digits.
func isAlphaNumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// isPlausibleAppName returns true if s looks like a real program/process name
// rather than a data fragment that happened to start in the TAG position.
//
// Many network appliances (PAN-OS, Cisco FTD) emit BSD syslog without a
// traditional TAG field. The alphanumeric scan in parseBSDTag then picks up
// whatever data follows the hostname—typically a CSV version number ("1") or
// the year portion of an inline ISO timestamp ("2026"). These are not program
// names and should not be promoted to appname/source/service.
//
// Rejected patterns:
//   - Single non-letter character (e.g. the digit "1" from a PAN-OS FUTURE_USE
//     field); a single letter is accepted as a short program name (e.g. "q")
//   - Purely numeric (e.g. "2026" from an ISO 8601 timestamp prefix)
//   - Contains characters outside the set [a-zA-Z0-9._/@-] that are valid
//     in Unix process names (e.g. commas from CSV data, "=" from key=value)
func isPlausibleAppName(s string) bool {
	if len(s) == 0 {
		return false
	}
	// A single character is only a plausible program name when it is a letter
	// (e.g. a short daemon tag "q:"). A lone digit or punctuation char is a
	// CSV/FUTURE_USE data fragment (e.g. PAN-OS "1"), not a real TAG.
	if len(s) == 1 {
		c := s[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	allDigit := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			allDigit = false
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		switch c {
		case '.', '_', '-', '/', '@':
			allDigit = false
			continue
		}
		return false
	}
	return !allDigit
}

// looksLikeISOTimestamp returns true if b starts with an ISO 8601 date prefix
// (YYYY-MM or YYYY-). This detects the "double-header" pattern used by devices
// like Cisco FTD, which embed a second timestamp after the BSD hostname:
//
//	<PRI>Mmm dd hh:mm:ss MGMT_IP YYYY-MM-DDThh:mm:ssZ hostname (tag) %ID: ...
//
// When this pattern appears in the TAG position, no real TAG is present and the
// entire remainder should be treated as MSG.
func looksLikeISOTimestamp(b []byte) bool {
	if len(b) < 5 {
		return false
	}
	return b[0] >= '0' && b[0] <= '9' &&
		b[1] >= '0' && b[1] <= '9' &&
		b[2] >= '0' && b[2] <= '9' &&
		b[3] >= '0' && b[3] <= '9' &&
		b[4] == '-'
}

// ---------------------------------------------------------------------------
// STRUCTURED-DATA parsing (RFC 5424)
// ---------------------------------------------------------------------------

// parseStructuredData parses STRUCTURED-DATA at the start of b.
// Returns the parsed SD elements as a map (SD-ID -> params), the byte length
// consumed (needed to locate the MSG field), and any error.
// NILVALUE ("-") returns (nil, 1, nil).
func parseStructuredData(b []byte) (map[string]map[string]string, int, error) {
	if len(b) == 0 {
		return nil, 0, errSDEmpty
	}
	if b[0] == '-' {
		return nil, 1, nil
	}
	if b[0] != '[' {
		return nil, 0, errSDExpected
	}
	result := make(map[string]map[string]string)
	i := 0
	for i < len(b) && b[i] == '[' {
		sdID, params, end, err := parseSDElement(b, i)
		if err != nil {
			if len(result) > 0 {
				return result, i, err
			}
			return nil, 0, err
		}
		result[sdID] = params
		i = end
	}
	return result, i, nil
}

// parseSDElement parses a single [ SD-ID *(SP SD-PARAM) ] starting at b[pos].
// Returns the SD-ID, the params map, and the index one past the closing ']'.
func parseSDElement(b []byte, pos int) (string, map[string]string, int, error) {
	if pos >= len(b) || b[pos] != '[' {
		return "", nil, 0, errSDElemOpen
	}
	i := pos + 1

	// SD-ID: 1*32 PRINTUSASCII except '=', SP, ']', '"'
	idStart := i
	for i < len(b) {
		c := b[i]
		if c == ' ' || c == ']' {
			break
		}
		if c == '=' || c == '"' || c < printUSASCIIMin || c > printUSASCIIMax {
			return "", nil, 0, errSDIDInvalid
		}
		i++
		if i-idStart > maxSDNameLen {
			return "", nil, 0, errSDIDTooLong
		}
	}
	if i == idStart {
		return "", nil, 0, errSDIDRequired
	}
	sdID := string(b[idStart:i])
	params := make(map[string]string)

	// *(SP SD-PARAM) then ']'
	for i < len(b) {
		if b[i] == ']' {
			return sdID, params, i + 1, nil
		}
		if b[i] != ' ' {
			return "", nil, 0, errSDElemExpect
		}
		i++ // skip SP
		name, value, end, err := parseSDParam(b, i)
		if err != nil {
			return "", nil, 0, err
		}
		params[name] = value
		i = end
	}
	return "", nil, 0, errSDElemUnclosed
}

// parseSDParam parses PARAM-NAME "=" '"' PARAM-VALUE '"' starting at b[pos].
// Returns PARAM-NAME, the unescaped PARAM-VALUE, and the index one past the
// closing '"'.
func parseSDParam(b []byte, pos int) (string, string, int, error) {
	if pos >= len(b) {
		return "", "", 0, errSDParamEmpty
	}
	i := pos

	// PARAM-NAME: 1*32 PRINTUSASCII except '=', SP, ']', '"'
	for i < len(b) && b[i] != '=' {
		c := b[i]
		if c == ' ' || c == ']' || c == '"' || c < printUSASCIIMin || c > printUSASCIIMax {
			return "", "", 0, errSDParamNameBad
		}
		i++
		if i-pos > maxSDNameLen {
			return "", "", 0, errSDParamNameLong
		}
	}
	if i == pos || i >= len(b) {
		return "", "", 0, errSDParamNoEq
	}
	name := string(b[pos:i])
	i++ // skip '='
	if i >= len(b) || b[i] != '"' {
		return "", "", 0, errSDParamNoQuote
	}
	i++ // skip opening '"'

	// Scan PARAM-VALUE: find closing '"', handle escapes.
	// Track whether any escapes are present to avoid allocation when possible.
	valStart := i
	hasEscape := false
	for i < len(b) {
		c := b[i]
		if c == '\\' {
			if i+1 >= len(b) {
				return "", "", 0, errSDParamTrailBS
			}
			hasEscape = true
			i += 2 // skip backslash + next byte
			continue
		}
		if c == '"' {
			var value string
			if hasEscape {
				value = unescapeSDValue(b[valStart:i])
			} else {
				value = string(b[valStart:i])
			}
			return name, value, i + 1, nil
		}
		i++
	}
	return "", "", 0, errSDParamUnclosed
}

// unescapeSDValue removes RFC 5424 §6.3.3 escapes from a PARAM-VALUE.
// Valid escapes: \" -> ", \\ -> \, \] -> ]. Invalid escapes are treated
// as literal (the backslash is removed).
func unescapeSDValue(b []byte) string {
	buf := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			i++ // skip backslash, take next byte literally
			buf = append(buf, b[i])
		} else {
			buf = append(buf, b[i])
		}
	}
	return string(buf)
}

// stripBOM removes the UTF-8 BOM (%xEF.BB.BF) if present.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// ---------------------------------------------------------------------------
// Shared helpers for structured message construction
// ---------------------------------------------------------------------------

// SeverityToStatus maps a syslog PRI value to an agent log status.
// Syslog severity is Pri % 8 per RFC 5424. The agent defines matching
// status constants in pkg/logs/message/status.go.
func SeverityToStatus(pri int) string {
	if pri < 0 {
		return message.StatusInfo
	}
	switch pri % 8 {
	case 0:
		return message.StatusEmergency
	case 1:
		return message.StatusAlert
	case 2:
		return message.StatusCritical
	case 3:
		return message.StatusError
	case 4:
		return message.StatusWarning
	case 5:
		return message.StatusNotice
	case 6:
		return message.StatusInfo
	case 7:
		return message.StatusDebug
	}
	return message.StatusInfo // unreachable: pri%8 covers 0-7
}
