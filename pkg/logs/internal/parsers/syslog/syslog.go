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

// maxCiscoTokenLen bounds the FACILITY and MNEMONIC halves of a Cisco message
// identifier so the scan cannot run away over arbitrary CONTENT.
const maxCiscoTokenLen = 32

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

	// Cisco's EMBLEM dialect terminates the PRI with a colon rather than going
	// straight into the header: the default remote-logging format on NX-OS is
	// "<189>:2025 Mar 27 16:22:24 switch %SYSLOG-...", and ASA emits the same
	// shape when EMBLEM is enabled. The colon carries nothing, so step over it —
	// but only when a timestamp follows, so that CONTENT which merely begins
	// with a colon is still treated as MSG per RFC 3164 §4.3.2.
	if pos+1 < len(line) && line[pos] == ':' {
		if n, _ := timestampLen(line[pos+1:]); n > 0 {
			pos++
		}
	}

	b := line[pos]
	switch {
	case b >= '0' && b <= '9':
		// A digit after PRI *may* be an RFC 5424 VERSION, but many network
		// appliances (PAN-OS, Cisco) emit no-timestamp BSD messages whose
		// CONTENT starts with a digit (e.g. "<134>1,2026/06/23,...,TRAFFIC,...").
		// Only dispatch to the RFC 5424 parser when the bytes actually form a
		// VERSION token (1-3 digits) followed by SP; otherwise treat the
		// remainder as MSG CONTENT per RFC 3164 §4.3.2.
		digitTS, _ := timestampLen(line[pos:])
		switch {
		case digitTS > 0:
			// Two layouts lead with a digit, and both must be tested before
			// isRFC5424Header or their leading digits are read as a VERSION:
			// the year-first BSD form "YYYY Mmm DD HH:MM:SS host
			// %FAC-SEV-MNEMONIC:" that is the NX-OS default, and the bare ISO
			// 8601 timestamp that Cisco ASA and FTD send under "logging
			// timestamp rfc5424" (as does Picus) with no VERSION at all.
			msg, err = parseBSD(line, pri, pos)
		case isRFC5424Header(line, pos):
			msg, err = parseRFC5424(line, pri, pos)
		default:
			msg = parseBSDNoTimestamp(line, pri, pos)
		}
	case (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z'):
		msg, err = parseBSD(line, pri, pos)
	case b == '*':
		// Cisco IOS marks an unsynchronized clock with '*' before the
		// TIMESTAMP. parseBSD steps over it; if no timestamp follows it falls
		// back to treating the remainder as MSG.
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

	// --- Split the 5 HEADER fields that follow VERSION ---
	// Fields: TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP ...
	//
	// RFC 5424 §6 mandates exactly one SP between fields, but emitters pad,
	// most often between VERSION and TIMESTAMP ("<134>1  2025-05-13T04:57:18Z
	// host ..."). No HEADER field may contain a space, so a run of them can be
	// collapsed without ever merging two fields.
	//
	// A field only counts as found once it is terminated by a space, which is
	// what keeps the best-effort recovery below aligned with a truncated
	// header. nextStart tracks where the next field begins, so an incomplete
	// header hands everything from that offset to MSG.
	var fields [5][]byte
	found := 0
	i := sp
	for i < len(line) && line[i] == ' ' {
		i++
	}
	nextStart := i
	for found < 5 && i < len(line) {
		start := i
		for i < len(line) && line[i] != ' ' {
			i++
		}
		if i >= len(line) {
			break // no terminating SP: the header is truncated here
		}
		fields[found] = line[start:i]
		for i < len(line) && line[i] == ' ' {
			i++
		}
		found++
		nextStart = i
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

	// Take whichever fields the header actually carried; the rest keep the
	// nilvalue default. A header that ran out early is Partial, and everything
	// from the first unparsed byte becomes MSG.
	if found > 0 {
		msg.Timestamp = toHeaderString(fields[0])
	}
	if found > 1 {
		msg.Hostname = toHeaderString(fields[1])
	}
	if found > 2 {
		msg.AppName = toHeaderString(fields[2])
	}
	if found > 3 {
		msg.ProcID = toHeaderString(fields[3])
	}
	if found > 4 {
		msg.MsgID = toHeaderString(fields[4])
	}
	if found < 5 {
		if nextStart < len(line) {
			msg.Msg = line[nextStart:]
		}
		msg.Partial = true
		return msg, errHeaderTooShort
	}

	// --- Parse STRUCTURED-DATA ---
	rest := line[nextStart:]
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
	// Accepted layouts, in order: the RFC 3164 15-byte form "Mmm dd hh:mm:ss",
	// the 14-byte non-padded-day and 20-byte with-year and year-first variants
	// emitted by various appliances, and finally an ISO 8601 timestamp, which
	// Cisco ASA/FTD and Picus send in place of the BSD form. The ISO layout also
	// turns up in tailed files regardless of sender, because rsyslog's default
	// file template renders the timestamp as RFC 3339 and drops the PRI.
	// Cisco IOS writes '*' ahead of the TIMESTAMP when the system clock has
	// never synchronized to a reliable source. It is a marker rather than part
	// of the time, so step over it and record the timestamp itself; the '*'
	// stays visible in the content, which is transmitted verbatim. Only skip it
	// when a timestamp actually follows, so a body opening with '*' is left as
	// MSG.
	if pos < len(line) && line[pos] == '*' {
		if n, _ := timestampLen(line[pos+1:]); n > 0 {
			pos++
		}
	}

	tsLen, isISOTimestamp := timestampLen(line[pos:])
	if !isISOTimestamp && tsLen > 0 {
		// "show-timezone" appends the zone name to the BSD layouts. It belongs
		// to the timestamp, not to the TAG position it occupies. A zone opens
		// with an uppercase letter and an ordinary HOSTNAME almost never does,
		// so that byte is tested before the call rather than inside it.
		if end := pos + tsLen; end+1 < len(line) &&
			line[end] == ' ' && line[end+1] >= 'A' && line[end+1] <= 'Z' {
			tsLen += bsdZoneSuffixLen(line[end:])
		}
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

	// Cisco ASA/FTD terminate the timestamp with a colon and omit both HOSTNAME
	// and TAG: "<PRI>2025-11-25T07:19:40Z: %ASA-4-733100: ...". There is no
	// hostname to read, so the remainder after the colon is CONTENT.
	if isISOTimestamp && pos < len(line) && line[pos] == ':' {
		pos++
		if pos < len(line) && line[pos] == ' ' {
			pos++
		}
		if pos < len(line) {
			msg.Msg = line[pos:]
		}
		return msg, nil
	}

	// Cisco devices omit the Device-ID when "logging device-id" is disabled,
	// leaving only a separator between the TIMESTAMP and the message mnemonic.
	// Its shape varies by platform and release:
	//
	//	2024-01-05T05:45:16Z   %FTD-1-430003: EventPriority: Low, ...
	//	May 17 2023 03:04:28: %ASA-6-302013: Built outbound TCP connection ...
	//
	// Cisco's syslog guide tells collectors to expect an optional colon
	// "followed by zero or more spaces" at this position, so neither the gap
	// width nor the colon carries meaning. There is no HOSTNAME to read: the
	// mnemonic begins the CONTENT.
	if sep := skipCiscoSeparator(line, pos); sep > pos && startsWithCiscoMnemonic(line[sep:]) {
		msg.Msg = line[sep:]
		return msg, nil
	}

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

	// Many emitters drop the HOSTNAME and put the TAG straight after the
	// TIMESTAMP — BIND writes "2024-09-20T10:20:26.751Z network: info: ..."
	// under print-category, and yum writes "Aug 20 12:51:21 Erased: pkg".
	// A trailing colon tells the two apart, since a hostname or IP address
	// never ends in one. Hand the whole remainder to the TAG parser, which
	// applies its own plausibility checks before committing an APP-NAME.
	if isTagShapedToken(line[pos:hostEnd]) {
		parseBSDTag(&msg, line[pos:])
		return msg, nil
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

	// Detect "double-header" formats where a second timestamp appears in the
	// TAG position. A relay that prepends its own header leaves the original
	// device header in place, so what follows the relay's HOSTNAME is another
	// TIMESTAMP rather than a TAG — in ISO form (Cisco FTD:
	// "YYYY-MM-DDThh:mm:ssZ hostname ...") or in BSD form (Cisco ISE:
	// "Mmm dd hh:mm:ss hostname CISE_..."). No real TAG is present, so the
	// entire remainder is MSG and the month abbreviation is not an APP-NAME.
	if looksLikeISOTimestamp(rest) || bsdTimestampLen(rest) > 0 {
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
		// No delimiter, so nothing marks where a TAG would end. RFC 3164
		// §4.3.3 only recognizes a TAG by its terminator; without one the
		// remainder is CONTENT.
		msg.Msg = rest
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

// isValidBSDTimestampSingleSpaceDay checks the 14-byte variant "Mmm d hh:mm:ss",
// where a single-digit day is written with one space instead of the space-padded
// "Mmm  d" that RFC 3164 requires:
//
//	RFC 3164:      "Jan  9 03:47:40" (15 bytes, day space-padded)
//	Non-padded:    "Jan 9 03:47:40"  (14 bytes)
//
// The non-padded form is common enough in the field that syslog-ng carries a
// dedicated detector for it and grok, Fluentd, and Vector all accept one or two
// spaces; it shows up here in Infoblox, Cisco ISE, and NX-OS samples.
func isValidBSDTimestampSingleSpaceDay(b []byte) bool {
	if len(b) < 14 {
		return false
	}
	if b[3] != ' ' || b[5] != ' ' || b[8] != ':' || b[11] != ':' {
		return false
	}
	if !isDigit(b[4]) {
		return false
	}
	return isValidMonthAbbrev(b)
}

// isValidBSDTimestampYearFirst checks the 20-byte variant "YYYY Mmm DD HH:MM:SS"
// emitted by Cisco NX-OS platforms, which place the year before the month
// instead of after the day:
//
//	RFC 3164:    "Apr  4 08:05:06"      (15 bytes)
//	With year:   "Apr 04 2024 08:05:06" (20 bytes)
//	Year first:  "2024 Apr 04 08:05:06" (20 bytes)
func isValidBSDTimestampYearFirst(b []byte) bool {
	if len(b) < 20 {
		return false
	}
	if b[4] != ' ' || b[8] != ' ' || b[11] != ' ' || b[14] != ':' || b[17] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 2, 3} {
		if !isDigit(b[i]) {
			return false
		}
	}
	// The day may be space-padded ("Apr  4") or zero-padded ("Apr 04").
	if !(isDigit(b[9]) || b[9] == ' ') || !isDigit(b[10]) {
		return false
	}
	return isValidMonthAbbrev(b[5:])
}

// bsdTimestampLen returns the length of a BSD-style timestamp at the start of b,
// trying every accepted layout, or 0 if b does not start with one. The 15-byte
// RFC 3164 form is tested before the 14-byte non-padded form so a conformant
// timestamp is never matched by the looser pattern.
func bsdTimestampLen(b []byte) int {
	// 14 bytes is the shortest accepted layout.
	if len(b) < 14 {
		return 0
	}
	// Year-first is the only layout that opens with a digit; every other one
	// opens with a month abbreviation. One look at the first byte therefore
	// picks the family, and the abbreviation is validated once rather than
	// once per layout.
	n := 0
	if isDigit(b[0]) {
		if !isValidBSDTimestampYearFirst(b) {
			return 0
		}
		n = 20
	} else {
		if !isValidMonthAbbrev(b) {
			return 0
		}
		switch {
		case isValidBSDTimestamp(b):
			n = 15
		case isValidBSDTimestampSingleSpaceDay(b):
			n = 14
		case isValidBSDTimestampWithYear(b):
			n = 20
		default:
			return 0
		}
	}
	// Every layout ends in seconds, so an optional fraction attaches the same
	// way to all of them. The '.' is tested here so the common case of a
	// timestamp without one costs a single comparison.
	if len(b) > n && b[n] == '.' {
		n += bsdFractionLen(b[n:])
	}
	return n
}

// bsdFractionLen returns the length of a fractional-seconds suffix — '.' and at
// least one digit — at the start of b, or 0 if there is none. Cisco IOS appends
// one under "service timestamps log datetime msec" ("Mar 18 14:52:10.039"),
// which its own documentation uses throughout. Every BSD layout ends in seconds,
// so the suffix attaches the same way to all of them, and isoTimestampLen
// already accepts the equivalent on the ISO layouts.
func bsdFractionLen(b []byte) int {
	if len(b) < 2 || b[0] != '.' || !isDigit(b[1]) {
		return 0
	}
	i := 2
	for i < len(b) && isDigit(b[i]) {
		i++
	}
	return i
}

// maxZoneAbbrevLen bounds a timezone abbreviation. Four letters covers the
// longest in common use ("AEDT", "CEST"); five leaves room to spare.
const maxZoneAbbrevLen = 5

// bsdZoneSuffixLen returns the length of a " ZONE" suffix following a BSD
// TIMESTAMP, or 0 if there is none. Cisco IOS appends the zone name under
// "service timestamps log datetime show-timezone", putting it exactly where a
// TAG would sit:
//
//	Mar  1 18:46:11 UTC: %LINK-3-UPDOWN: Interface Serial0 up
//
// Read as a TAG it becomes the APP-NAME, which then drives service routing from
// a timezone. A bare uppercase token is far too weak a signal to act on, so the
// suffix only counts when a Cisco mnemonic follows the colon that ends it —
// leaving a genuine uppercase TAG ("GW: session opened") alone. The colon itself
// is left in place for skipCiscoSeparator.
func bsdZoneSuffixLen(b []byte) int {
	// A lowercase byte here is the overwhelmingly common case — an ordinary
	// HOSTNAME — so it is rejected before anything else runs.
	if len(b) < 2 || b[0] != ' ' || b[1] < 'A' || b[1] > 'Z' {
		return 0
	}
	i := 1
	for i < len(b) && i-1 < maxZoneAbbrevLen && b[i] >= 'A' && b[i] <= 'Z' {
		i++
	}
	if i-1 < 2 || i >= len(b) || b[i] != ':' {
		return 0
	}
	rest := b[i+1:]
	for len(rest) > 0 && rest[0] == ' ' {
		rest = rest[1:]
	}
	if !startsWithCiscoMnemonic(rest) {
		return 0
	}
	return i
}

// timestampLen returns the length of the TIMESTAMP at the start of b and
// whether it is the ISO 8601 form rather than one of the BSD layouts, or 0 if
// b does not begin with a timestamp at all. Callers that only need to know
// whether a timestamp is present should use this rather than consulting the
// two detectors separately, so that dispatch and parsing cannot disagree about
// where a header starts.
func timestampLen(b []byte) (int, bool) {
	if n := bsdTimestampLen(b); n > 0 {
		return n, false
	}
	if n := isoTimestampLen(b); n > 0 {
		return n, true
	}
	return 0, false
}

// isoTimestampLen returns the length of an ISO 8601 / RFC 3339 timestamp at the
// start of b, or 0 if b does not start with one. The recognized shape is
//
//	YYYY-MM-DDThh:mm:ss[.frac][Z|(+|-)hh:mm|(+|-)hhmm]
//
// Only 'T' is accepted as the date/time separator: a space separator cannot be
// told apart from a date followed by an unrelated field, and accepting it would
// let the parser consume a neighbouring token.
func isoTimestampLen(b []byte) int {
	const base = 19 // YYYY-MM-DDThh:mm:ss
	if len(b) < base {
		return 0
	}
	if b[4] != '-' || b[7] != '-' || b[10] != 'T' || b[13] != ':' || b[16] != ':' {
		return 0
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if !isDigit(b[i]) {
			return 0
		}
	}
	n := base

	// Optional fractional seconds: '.' followed by at least one digit.
	if n < len(b) && b[n] == '.' {
		f := n + 1
		for f < len(b) && isDigit(b[f]) {
			f++
		}
		if f > n+1 {
			n = f
		}
	}

	// Optional zone: 'Z', "+hh:mm"/"-hh:mm", or "+hhmm"/"-hhmm".
	if n < len(b) {
		switch b[n] {
		case 'Z', 'z':
			n++
		case '+', '-':
			switch {
			case n+6 <= len(b) && isDigit(b[n+1]) && isDigit(b[n+2]) &&
				b[n+3] == ':' && isDigit(b[n+4]) && isDigit(b[n+5]):
				n += 6
			case n+5 <= len(b) && isDigit(b[n+1]) && isDigit(b[n+2]) &&
				isDigit(b[n+3]) && isDigit(b[n+4]):
				n += 5
			}
		}
	}
	return n
}

// isDigit reports whether b is an ASCII decimal digit.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
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

// isTagShapedToken reports whether the token sitting in the HOSTNAME position
// is really a TAG. The discriminator is a single trailing colon: RFC 3164
// §4.1.2 defines HOSTNAME as a hostname or IP address, neither of which ends
// in a colon, while RFC 3164 §4.1.3 terminates the TAG with one.
//
// Tokens holding more than one colon are deliberately left as HOSTNAME so an
// IPv6 address that ends in "::" is not mistaken for a TAG.
func isTagShapedToken(tok []byte) bool {
	if len(tok) < 2 || tok[len(tok)-1] != ':' {
		return false
	}
	colons := 0
	for _, c := range tok {
		if c == ':' {
			colons++
		}
	}
	return colons == 1
}

// looksLikeISOTimestamp returns true if b starts with an ISO 8601 date prefix
// (YYYY-MM or YYYY-). This detects the "double-header" pattern used by devices
// like Cisco FTD, which embed a second timestamp after the BSD hostname:
//
//	<PRI>Mmm dd hh:mm:ss MGMT_IP YYYY-MM-DDThh:mm:ssZ hostname (tag) %ID: ...
//
// When this pattern appears in the TAG position, no real TAG is present and the
// entire remainder should be treated as MSG.
//
// The four-digit-and-dash prefix is all this checks, which is deliberately
// weaker than isoTimestampLen: the embedded timestamp is never consumed, only
// recognized, so a relay writing a date-only or otherwise non-conformant second
// header still keeps its remainder out of APP-NAME. Use isoTimestampLen instead
// wherever the match decides how many bytes to advance.
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

// skipCiscoSeparator advances past the spaces and at most one colon that Cisco
// devices place between the TIMESTAMP (or Device-ID) and the message mnemonic.
// Both parts are optional: FTD pads with spaces when the Device-ID is
// disabled, while ASA under "logging timestamp" writes the colon straight
// after the timestamp. It returns pos unchanged when neither is present, so
// callers can tell whether a separator was consumed at all.
func skipCiscoSeparator(line []byte, pos int) int {
	i := pos
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i < len(line) && line[i] == ':' {
		i++
		for i < len(line) && line[i] == ' ' {
			i++
		}
	}
	return i
}

// startsWithCiscoMnemonic reports whether b opens with a Cisco message
// identifier — "%FACILITY-SEVERITY-MNEMONIC" — as emitted by ASA, FTD, NGIPS
// and NX-OS (e.g. "%FTD-1-430003:", "%ASA-6-302013:",
// "%ETHPORT-5-IF_DOWN_CFG_CHANGE:"). SEVERITY is the single syslog severity
// digit, which is what keeps the token unambiguous: RFC 3164 §4.1.2 defines
// HOSTNAME as a hostname or IP address, so neither the leading '%' nor the
// interior severity digit can occur in a well-formed value here.
func startsWithCiscoMnemonic(b []byte) bool {
	if len(b) < 2 || b[0] != '%' {
		return false
	}
	i := 1

	facility := i
	for i < len(b) && (isAlphaNumeric(b[i]) || b[i] == '_') {
		i++
	}
	if i == facility || i-facility > maxCiscoTokenLen {
		return false
	}
	if i >= len(b) || b[i] != '-' {
		return false
	}
	i++

	if i >= len(b) || b[i] < '0' || b[i] > '7' {
		return false
	}
	i++
	if i >= len(b) || b[i] != '-' {
		return false
	}
	i++

	mnemonic := i
	for i < len(b) && (isAlphaNumeric(b[i]) || b[i] == '_') {
		i++
	}
	return i > mnemonic && i-mnemonic <= maxCiscoTokenLen
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
