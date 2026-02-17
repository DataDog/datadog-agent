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
// Fatal errors (empty input, no PRI) still return a zero SyslogMessage.
//
// The parser is allocation-light: it returns SyslogMessage by value (no heap
// escape) and string fields are standard Go strings (independent copies of the
// input). The caller is free to reuse or discard the input []byte immediately.
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

	errNoSPAfterVersion = errors.New("header too short: no SP after VERSION")

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
	errBSDTimestamp  = errors.New("BSD: invalid timestamp format")
	errBSDMonth      = errors.New("BSD: unrecognized month abbreviation")
	errBSDHostname   = errors.New("BSD: missing hostname")
	errUnknownFormat = errors.New("unknown format: expected digit (RFC 5424) or letter (BSD) after PRI")
)

const nilvalue = "-"

// SyslogMessage is a parsed syslog message (RFC 5424 or RFC 3164/BSD).
//
// String fields are independent copies; the caller may freely reuse or
// discard the input []byte after Parse returns.
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
		return SyslogMessage{}, errEmpty
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
		return SyslogMessage{}, errNoPRIClose
	}

	// --- Validate PRI ---
	if gtPos < 2 || line[0] != '<' {
		return SyslogMessage{}, errPRIFormat
	}
	pri := 0
	for i := 1; i < gtPos; i++ {
		d := line[i]
		if d < '0' || d > '9' {
			return SyslogMessage{}, errPRINonDigit
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
		return SyslogMessage{}, errNoSPAfterVersion
	}

	var msg SyslogMessage
	var err error

	b := line[pos]
	switch {
	case b >= '1' && b <= '9':
		msg, err = parseRFC5424(line, pri, pos)
	case (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z'):
		msg, err = parseBSD(line, pri, pos)
	default:
		return SyslogMessage{}, errUnknownFormat
	}

	// Apply PRI range warning if the downstream parse didn't report a worse error.
	if priErr != nil {
		msg.Partial = true
		if err == nil {
			err = priErr
		}
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
		return SyslogMessage{}, errEmpty
	}
	return parseBSD(line, -1, 0)
}

// ---------------------------------------------------------------------------
// RFC 5424 parsing
// ---------------------------------------------------------------------------

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
		return SyslogMessage{}, errNoSPAfterVersion
	}
	verRaw := line[pos:sp]

	// Validate VERSION: NONZERO-DIGIT 0*2DIGIT
	if len(verRaw) == 0 {
		return SyslogMessage{}, errVersionEmpty
	}
	if verRaw[0] < '1' || verRaw[0] > '9' {
		return SyslogMessage{}, errVersionStart
	}
	if len(verRaw) > 3 {
		return SyslogMessage{}, errVersionLen
	}
	for i := 1; i < len(verRaw); i++ {
		if verRaw[i] < '0' || verRaw[i] > '9' {
			return SyslogMessage{}, errVersionDigit
		}
	}

	// --- Find SPs after VERSION to delimit header fields ---
	// Fields: TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP ...
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
		// Best-effort: treat the entire rest region as MSG.
		msg.Msg = rest
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

// parseBSD parses a BSD syslog message starting at line[pos]. PRI has already
// been extracted (pri=-1 for file input where PRI is absent).
//
// Best-effort: extracts TIMESTAMP, HOSTNAME, TAG (APP-NAME + PROCID), and
// CONTENT (MSG) in order. On failure at any step, remaining fields get
// defaults and unparsed data goes into Msg.
func parseBSD(line []byte, pri int, pos int) (SyslogMessage, error) {
	msg := SyslogMessage{
		Pri:     pri,
		MsgID:   nilvalue,
		ProcID:  nilvalue,
		AppName: nilvalue,
	}

	// --- TIMESTAMP: exactly 15 bytes ---
	// Format: Mmm dd hh:mm:ss  (or Mmm  d hh:mm:ss for single-digit day)
	if pos+15 > len(line) {
		if pos < len(line) {
			msg.Msg = line[pos:]
		}
		msg.Partial = true
		return msg, errBSDTimestamp
	}

	tsRaw := line[pos : pos+15]
	if !isValidMonth(tsRaw) {
		// Invalid month — stuff everything into MSG.
		msg.Msg = line[pos:]
		msg.Partial = true
		return msg, errBSDMonth
	}
	msg.Timestamp = string(tsRaw)
	pos += 15

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
		msg.AppName = string(rest)
		return
	}

	switch delimChar {
	case '[':
		// appname[pid]: content
		msg.AppName = string(rest[:delimIdx])

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
		// appname: content
		msg.AppName = string(rest[:delimIdx])

		contentStart := delimIdx + 1
		if contentStart < len(rest) && rest[contentStart] == ' ' {
			contentStart++
		}
		if contentStart < len(rest) {
			msg.Msg = rest[contentStart:]
		}

	case ' ':
		// appname content (no colon or bracket delimiter)
		msg.AppName = string(rest[:delimIdx])

		contentStart := delimIdx + 1
		if contentStart < len(rest) {
			msg.Msg = rest[contentStart:]
		}
	}
}

// isValidMonth checks if the first 3 bytes of b are a valid BSD month
// abbreviation (Jan, Feb, Mar, Apr, May, Jun, Jul, Aug, Sep, Oct, Nov, Dec).
func isValidMonth(b []byte) bool {
	if len(b) < 3 {
		return false
	}
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
		if c == '=' || c == '"' {
			return "", nil, 0, errSDIDInvalid
		}
		i++
		if i-idStart > 32 {
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
		if c == ' ' || c == ']' || c == '"' {
			return "", "", 0, errSDParamNameBad
		}
		i++
		if i-pos > 32 {
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
	default:
		return message.StatusInfo
	}
}

// BuildSyslogFields returns the syslog metadata map for a parsed message.
// Used by both the TCP tailer's builder and the file parser to construct
// the "syslog" sub-map in the structured content.
//
// Fields that are absent or empty are omitted:
//   - Pri < 0: severity and facility are omitted
//   - Version == "": version is omitted (BSD messages)
//   - StructuredData == nil: structured_data is omitted (BSD messages)
func BuildSyslogFields(parsed SyslogMessage) map[string]interface{} {
	fields := map[string]interface{}{
		"timestamp": parsed.Timestamp,
		"hostname":  parsed.Hostname,
		"appname":   parsed.AppName,
		"procid":    parsed.ProcID,
		"msgid":     parsed.MsgID,
	}
	if parsed.Pri >= 0 {
		fields["severity"] = parsed.Pri % 8
		fields["facility"] = parsed.Pri / 8
	}
	if parsed.Version != "" {
		fields["version"] = parsed.Version
	}
	if parsed.StructuredData != nil {
		fields["structured_data"] = parsed.StructuredData
	}
	return fields
}
