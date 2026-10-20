// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits are per invocation, including inputs that never reach EOF.
const (
	MaxFilterBytes     = 64 << 10
	MaxTotalInputBytes = 8 << 20
	MaxRawLineBytes    = 1 << 20
	MaxNestingDepth    = 64
	MaxFilterNodes     = 4_096
	MaxValueNodes      = 65_536
	MaxValueBytes      = 4 << 20
	MaxIntegerBits     = 256
	MaxEvalSteps       = 100_000
	MaxResults         = 10_000
	MaxResultNodes     = 2 * MaxValueNodes
	MaxResultBytes     = 8 << 20
	MaxOutputBytes     = 1 << 20

	maxNumberBytes = 256
	// Invalid UTF-8 can expand threefold when normalized to U+FFFD.
	maxNormalizedJSONBytes = 3 * MaxValueBytes
)

var (
	errIntegerRange = errors.New("integer exceeds the supported 256-bit range")
	errValueNodes   = errors.New("JSON value contains too many nodes")
	errValueBytes   = errors.New("JSON value exceeds the size limit")
	errValueDepth   = errors.New("JSON value is nested too deeply")
	errOutputLimit  = errors.New("generated output exceeds the size limit")
)

type valueKind uint8

const (
	valueNull valueKind = iota
	valueBoolean
	valueNumber
	valueString
	valueArray
	valueObject
)

type number struct {
	integer *big.Int
	float   float64
}

func (n number) isInteger() bool { return n.integer != nil }

func (n number) indexInt64() (int64, bool) {
	if n.isInteger() {
		if !n.integer.IsInt64() {
			return 0, false
		}
		return n.integer.Int64(), true
	}
	truncated := math.Trunc(n.float)
	if truncated < float64(math.MinInt64) || truncated >= -float64(math.MinInt64) {
		return 0, false
	}
	return int64(truncated), true
}

func (n number) text() string {
	if n.integer != nil {
		return n.integer.String()
	}
	return strconv.FormatFloat(n.float, 'g', -1, 64)
}

type objectMember struct {
	key   string
	value value
}

type objectValue struct {
	members []objectMember
	index   map[string]int
}

type value struct {
	kind  valueKind
	bool  bool
	num   number
	str   string
	array []value
	obj   objectValue
	nodes int
	bytes int
	depth int
}

func null() value {
	return value{kind: valueNull, nodes: 1, bytes: 4}
}

func boolean(b bool) value {
	size := 5
	if b {
		size = 4
	}
	return value{kind: valueBoolean, bool: b, nodes: 1, bytes: size}
}

func stringValue(s string) (value, error) {
	s = normalizeUTF8(s)
	if len(s)+2 > MaxValueBytes {
		return value{}, errValueBytes
	}
	return value{kind: valueString, str: s, nodes: 1, bytes: len(s) + 2}, nil
}

func normalizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var output strings.Builder
	output.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			output.WriteRune(utf8.RuneError)
			s = s[invalidUTF8UnitSize(s):]
			continue
		}
		output.WriteString(s[:size])
		s = s[size:]
	}
	return output.String()
}

func invalidUTF8UnitSize(s string) int {
	expected := 1
	switch {
	case s[0] >= 0xc2 && s[0] <= 0xdf:
		expected = 2
	case s[0] >= 0xe0 && s[0] <= 0xef:
		expected = 3
	case s[0] >= 0xf0 && s[0] <= 0xf4:
		expected = 4
	default:
		return 1
	}
	if len(s) < expected {
		return len(s)
	}
	for i := 1; i < expected; i++ {
		if s[i]&0xc0 != 0x80 {
			return i
		}
	}
	return expected
}

func integerValue(i *big.Int) (value, error) {
	if i == nil || i.BitLen() > MaxIntegerBits {
		return value{}, errIntegerRange
	}
	copyInt := new(big.Int).Set(i)
	text := copyInt.String()
	return value{
		kind:  valueNumber,
		num:   number{integer: copyInt},
		nodes: 1,
		bytes: len(text),
	}, nil
}

func int64Value(i int64) value {
	n := big.NewInt(i)
	return value{
		kind:  valueNumber,
		num:   number{integer: n},
		nodes: 1,
		bytes: len(n.String()),
	}
}

func addAggregate(nodes, size *int, item value, extra, maxNodes, maxBytes int) error {
	if item.nodes > maxNodes-*nodes {
		return errValueNodes
	}
	if extra > maxBytes || item.bytes > maxBytes-extra || *size > maxBytes-extra-item.bytes {
		return errValueBytes
	}
	*nodes += item.nodes
	*size += extra + item.bytes
	return nil
}

func floatValue(f float64) (value, error) {
	if math.IsNaN(f) {
		return value{}, errors.New("number is not finite")
	}
	// jq saturates overflowed doubles instead of failing.
	if math.IsInf(f, 0) {
		f = math.Copysign(math.MaxFloat64, f)
	}
	text := strconv.FormatFloat(f, 'g', -1, 64)
	return value{
		kind:  valueNumber,
		num:   number{float: f},
		nodes: 1,
		bytes: len(text),
	}, nil
}

func parseNumber(text string) (value, error) {
	if len(text) == 0 || len(text) > maxNumberBytes {
		return value{}, errors.New("number literal is too long")
	}
	if strings.ContainsAny(text, ".eE") {
		// ErrRange still returns a clampable infinity or an underflowed zero.
		f, err := strconv.ParseFloat(text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return value{}, fmt.Errorf("invalid number %q", text)
		}
		return floatValue(f)
	}
	i, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return value{}, fmt.Errorf("invalid number %q", text)
	}
	// big.Int cannot preserve jq's signed zero.
	if i.Sign() == 0 && strings.HasPrefix(text, "-") {
		return floatValue(math.Copysign(0, -1))
	}
	return integerValue(i)
}

func arrayValue(items []value) (value, error) {
	nodes, size, depth := 1, 2, 1
	for i := range items {
		separator := 0
		if i > 0 {
			separator = 1
		}
		if err := addAggregate(&nodes, &size, items[i], separator, MaxValueNodes, MaxValueBytes); err != nil {
			return value{}, err
		}
		if items[i].depth+1 > depth {
			depth = items[i].depth + 1
		}
	}
	if depth > MaxNestingDepth {
		return value{}, errValueDepth
	}
	return value{kind: valueArray, array: items, nodes: nodes, bytes: size, depth: depth}, nil
}

func object(items []objectMember) (value, error) {
	members := make([]objectMember, 0, len(items))
	index := make(map[string]int, len(items))
	for _, item := range items {
		if existing, ok := index[item.key]; ok {
			members[existing].value = item.value
			continue
		}
		index[item.key] = len(members)
		members = append(members, item)
	}

	nodes, size, depth := 1, 2, 1
	for i := range members {
		item := &members[i]
		separator := 0
		if i > 0 {
			separator = 1
		}
		if err := addAggregate(&nodes, &size, item.value, len(item.key)+3+separator, MaxValueNodes, MaxValueBytes); err != nil {
			return value{}, err
		}
		if item.value.depth+1 > depth {
			depth = item.value.depth + 1
		}
	}
	if depth > MaxNestingDepth {
		return value{}, errValueDepth
	}

	return value{
		kind:  valueObject,
		obj:   objectValue{members: members, index: index},
		nodes: nodes,
		bytes: size,
		depth: depth,
	}, nil
}

func (o objectValue) get(key string) (value, bool) {
	i, ok := o.index[key]
	if !ok {
		return value{}, false
	}
	return o.members[i].value, true
}

type jsonValueDecoder struct {
	ctx    context.Context
	dec    *json.Decoder
	stream *jsonStreamReader
}

func newJSONValueDecoder(ctx context.Context, r io.Reader) *jsonValueDecoder {
	stream := &jsonStreamReader{reader: r}
	dec := json.NewDecoder(stream)
	dec.UseNumber()
	return &jsonValueDecoder{ctx: ctx, dec: dec, stream: stream}
}

func (d *jsonValueDecoder) next() (value, error) {
	var raw json.RawMessage
	start := d.dec.InputOffset()
	d.stream.discardBefore(start)
	if err := d.dec.Decode(&raw); err != nil {
		if drainErr := d.drainUnterminatedToken(start); drainErr != nil {
			return value{}, drainErr
		}
		if ctxErr := d.ctx.Err(); ctxErr != nil {
			return value{}, ctxErr
		}
		return value{}, err
	}
	if len(raw) == 0 {
		return value{}, errors.New("empty JSON value")
	}
	if raw[0] != '{' && raw[0] != '[' && raw[0] != '"' {
		next, err := d.stream.peek(d.dec.InputOffset())
		if err != nil && !errors.Is(err, io.EOF) {
			return value{}, err
		}
		if err == nil && !isPrimitiveBoundary(next) {
			if isJQLiteralByte(next) {
				if drainErr := d.stream.drainJQLiteral(d.dec.InputOffset()); drainErr != nil {
					return value{}, drainErr
				}
			}
			return value{}, fmt.Errorf("invalid character %q after top-level value", next)
		}
	}
	if err := d.ctx.Err(); err != nil {
		return value{}, err
	}
	if err := validateSurrogatesBytes(raw); err != nil {
		return value{}, err
	}
	if err := d.ctx.Err(); err != nil {
		return value{}, err
	}
	normalizedRaw, err := normalizeJSONStrings(raw)
	if err != nil {
		return value{}, err
	}
	if err := d.ctx.Err(); err != nil {
		return value{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(normalizedRaw))
	decoder.UseNumber()
	return (&jsonValueDecoder{ctx: d.ctx, dec: decoder}).decode(0)
}

func (d *jsonValueDecoder) drainUnterminatedToken(offset int64) error {
	start, ok := d.stream.index(offset)
	if !ok {
		return errors.New("invalid JSON decoder offset")
	}
	if stringStart, escaped, ok := trailingUnterminatedJSONString(d.stream.delivered, start); ok {
		reachable, err := jsonPrefixCanReachToken(d.ctx, d.stream.delivered[start:stringStart], true)
		if err != nil {
			return err
		}
		if reachable {
			if err := d.stream.drainJSONString(escaped); err != nil {
				return err
			}
			return d.ctx.Err()
		}
		return nil
	}
	literal, ok := trailingJQLiteral(d.stream.delivered, start)
	if !ok {
		return d.drainAfterProvisionalObjectKey(offset)
	}
	reachable, err := jsonPrefixCanReachToken(d.ctx, d.stream.delivered[start:literal], false)
	if err != nil {
		return err
	}
	if !reachable {
		return nil
	}
	if err := d.stream.drainJQLiteral(d.stream.base + int64(literal)); err != nil {
		return err
	}
	return d.ctx.Err()
}

func (d *jsonValueDecoder) drainAfterProvisionalObjectKey(offset int64) error {
	start, ok := d.stream.index(offset)
	if !ok {
		return errors.New("invalid JSON decoder offset")
	}
	normalized, ok, err := normalizeProvisionalObjectKeys(d.ctx, d.stream.delivered[start:])
	if err != nil || !ok {
		return err
	}
	reachable, err := jsonPrefixReachesEOF(d.ctx, normalized, false)
	if err != nil || !reachable {
		return err
	}
	if err := d.stream.drainNextJQToken(d.ctx); err != nil {
		return err
	}
	return d.ctx.Err()
}

func trailingUnterminatedJSONString(data []byte, offset int) (int, bool, bool) {
	if offset < 0 || offset >= len(data) {
		return 0, false, false
	}
	inString := false
	escaped := false
	start := 0
	for i, c := range data[offset:] {
		if !inString {
			if c == '"' {
				inString = true
				escaped = false
				start = offset + i
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			inString = false
		}
	}
	return start, escaped, inString
}

func trailingJQLiteral(data []byte, offset int) (int, bool) {
	if offset < 0 || offset >= len(data) || !isJQLiteralByte(data[len(data)-1]) {
		return 0, false
	}
	start := len(data) - 1
	for start > offset && isJQLiteralByte(data[start-1]) {
		start--
	}
	if !outsideJSONStringAt(data[offset:], start-offset) {
		return 0, false
	}
	return start, true
}

func outsideJSONStringAt(data []byte, offset int) bool {
	inString := false
	escaped := false
	for _, c := range data[:offset] {
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			inString = false
		}
	}
	return !inString
}

func jsonPrefixCanReachToken(ctx context.Context, prefix []byte, terminatePrimitive bool) (bool, error) {
	reachable, err := jsonPrefixReachesEOF(ctx, prefix, terminatePrimitive)
	if err != nil || reachable {
		return reachable, err
	}
	normalized, ok, err := normalizeProvisionalObjectKeys(ctx, prefix)
	if err != nil || !ok {
		return false, err
	}
	return jsonPrefixReachesEOF(ctx, normalized, terminatePrimitive)
}

func jsonPrefixReachesEOF(ctx context.Context, prefix []byte, terminatePrimitive bool) (bool, error) {
	input := prefix
	if terminatePrimitive {
		input = make([]byte, len(prefix)+1)
		copy(input, prefix)
		input[len(prefix)] = ' '
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	for tokens := 0; ; tokens++ {
		if tokens%256 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), nil
		}
	}
}

type jsonContainerFrame struct {
	kind     byte
	keyStart int
}

func normalizeProvisionalObjectKeys(ctx context.Context, prefix []byte) ([]byte, bool, error) {
	frames, ok, err := jsonContainerFrames(ctx, prefix)
	if err != nil || !ok {
		return nil, false, err
	}
	insertions := make([]int, 0, len(frames))
	for _, frame := range frames {
		if frame.kind != '{' {
			continue
		}
		start := frame.keyStart
		for start < len(prefix) && isJSONWhitespace(prefix[start]) {
			start++
		}
		if start == len(prefix) || prefix[start] == '"' {
			continue
		}
		insertions = append(insertions, start)
	}
	if len(insertions) == 0 {
		return nil, false, nil
	}
	normalized := make([]byte, 0, len(prefix)+3*len(insertions))
	last := 0
	for _, insertion := range insertions {
		normalized = append(normalized, prefix[last:insertion]...)
		normalized = append(normalized, '"', '"', ':')
		last = insertion
	}
	normalized = append(normalized, prefix[last:]...)
	return normalized, true, nil
}

func jsonContainerFrames(ctx context.Context, data []byte) ([]jsonContainerFrame, bool, error) {
	stack := make([]jsonContainerFrame, 0, MaxNestingDepth)
	inString := false
	escaped := false
	for i, c := range data {
		if i%inputReadChunk == 0 {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			if len(stack) >= MaxNestingDepth {
				return nil, false, nil
			}
			frame := jsonContainerFrame{kind: c}
			if c == '{' {
				frame.keyStart = i + 1
			}
			stack = append(stack, frame)
		case ',':
			if len(stack) > 0 && stack[len(stack)-1].kind == '{' {
				stack[len(stack)-1].keyStart = i + 1
			}
		case '}', ']':
			if len(stack) == 0 || c == '}' && stack[len(stack)-1].kind != '{' || c == ']' && stack[len(stack)-1].kind != '[' {
				return nil, false, nil
			}
			stack = stack[:len(stack)-1]
		}
	}
	if inString {
		return nil, false, nil
	}
	return stack, true, nil
}

func isJSONWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func normalizeJSONStrings(raw []byte) ([]byte, error) {
	if utf8.Valid(raw) {
		return raw, nil
	}
	output := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] != '"' {
			if len(output) >= maxNormalizedJSONBytes {
				return nil, errValueBytes
			}
			output = append(output, raw[i])
			i++
			continue
		}
		end, err := jsonStringEnd(raw, i)
		if err != nil {
			return nil, err
		}
		token := raw[i:end]
		if utf8.Valid(token) {
			if len(token) > maxNormalizedJSONBytes-len(output) {
				return nil, errValueBytes
			}
			output = append(output, token...)
		} else {
			decoded, err := decodeJSONString(token)
			if err != nil {
				return nil, err
			}
			v, err := stringValue(string(decoded))
			if err != nil {
				return nil, err
			}
			// Use the remaining budget across all strings in the value.
			output, err = appendNormalizedJSONString(output, v.str, maxNormalizedJSONBytes)
			if err != nil {
				return nil, err
			}
		}
		i = end
	}
	return output, nil
}

// appendNormalizedJSONString emits JSON for reparsing without output-only DEL escaping.
func appendNormalizedJSONString(dst []byte, s string, limit int) ([]byte, error) {
	const hex = "0123456789abcdef"
	if len(dst) >= limit {
		return nil, errValueBytes
	}
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		if len(dst)+6 > limit {
			return nil, errValueBytes
		}
		switch c := s[i]; c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	if len(dst) >= limit {
		return nil, errValueBytes
	}
	return append(dst, '"'), nil
}

func jsonStringEnd(raw []byte, start int) (int, error) {
	escaped := false
	for i := start + 1; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch raw[i] {
		case '\\':
			escaped = true
		case '"':
			return i + 1, nil
		}
	}
	return 0, errors.New("unterminated JSON string")
}

func decodeJSONString(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, errors.New("invalid JSON string")
	}
	maxBytes := MaxValueBytes - 2
	decoded := make([]byte, 0, min(len(raw)-2, maxBytes))
	end := len(raw) - 1
	for i := 1; i < end; {
		c := raw[i]
		i++
		if c != '\\' {
			if c < 0x20 {
				return nil, errors.New("unescaped control character in JSON string")
			}
			if len(decoded) >= maxBytes {
				return nil, errValueBytes
			}
			decoded = append(decoded, c)
			continue
		}
		if i >= end {
			return nil, errors.New("incomplete JSON string escape")
		}
		escape := raw[i]
		i++
		switch escape {
		case '"', '\\', '/':
			decoded = append(decoded, escape)
		case 'b':
			decoded = append(decoded, '\b')
		case 'f':
			decoded = append(decoded, '\f')
		case 'n':
			decoded = append(decoded, '\n')
		case 'r':
			decoded = append(decoded, '\r')
		case 't':
			decoded = append(decoded, '\t')
		case 'u':
			codeUnit, ok := jsonHexCodeUnit(raw[i:end])
			if !ok {
				return nil, errors.New("invalid Unicode escape in JSON string")
			}
			i += 4
			codepoint := rune(codeUnit)
			if codeUnit >= 0xd800 && codeUnit <= 0xdbff {
				if end-i < 6 || raw[i] != '\\' || raw[i+1] != 'u' {
					return nil, errInvalidSurrogate
				}
				low, ok := jsonHexCodeUnit(raw[i+2 : end])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return nil, errInvalidSurrogate
				}
				i += 6
				codepoint = 0x10000 + (rune(codeUnit-0xd800) << 10) + rune(low-0xdc00)
			} else if codeUnit >= 0xdc00 && codeUnit <= 0xdfff {
				// jq decodes an unpaired low surrogate to U+FFFD.
				codepoint = utf8.RuneError
			}
			decoded = append(decoded, string(codepoint)...)
		default:
			return nil, errors.New("invalid JSON string escape")
		}
		if len(decoded) > maxBytes {
			return nil, errValueBytes
		}
	}
	return decoded, nil
}

func jsonHexCodeUnit(raw []byte) (uint16, bool) {
	if len(raw) < 4 {
		return 0, false
	}
	var codeUnit uint16
	for _, c := range raw[:4] {
		digit, ok := hexDigit(c)
		if !ok {
			return 0, false
		}
		codeUnit = codeUnit<<4 | digit
	}
	return codeUnit, true
}

type jsonStreamReader struct {
	reader io.Reader
	// delivered supports token-error recovery; base is its absolute offset.
	// Discarded prefixes are compacted in batches.
	base       int64
	delivered  []byte
	discarded  int
	pending    []byte
	pendingErr error
}

func (r *jsonStreamReader) index(offset int64) (int, bool) {
	i := offset - r.base
	if i < 0 || i > int64(len(r.delivered)) {
		return 0, false
	}
	return int(i), true
}

func (r *jsonStreamReader) discardBefore(offset int64) {
	i, ok := r.index(offset)
	if !ok || i == 0 {
		return
	}
	r.delivered = r.delivered[i:]
	r.base = offset
	r.discarded += i
	if r.discarded < inputReadChunk || r.discarded < len(r.delivered) {
		return
	}
	kept := make([]byte, len(r.delivered))
	copy(kept, r.delivered)
	r.delivered = kept
	r.discarded = 0
}

func (r *jsonStreamReader) Read(p []byte) (int, error) {
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.delivered = append(r.delivered, r.pending[:n]...)
		r.pending = r.pending[n:]
		if len(r.pending) == 0 {
			err := r.pendingErr
			r.pendingErr = nil
			return n, err
		}
		return n, nil
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.delivered = append(r.delivered, p[:n]...)
	}
	return n, err
}

func (r *jsonStreamReader) peek(offset int64) (byte, error) {
	i, ok := r.index(offset)
	if !ok {
		return 0, errors.New("invalid JSON decoder offset")
	}
	if i < len(r.delivered) {
		return r.delivered[i], nil
	}
	if len(r.pending) > 0 {
		return r.pending[0], nil
	}
	for {
		var one [1]byte
		n, err := r.reader.Read(one[:])
		if n > 0 {
			r.pending = append(r.pending, one[:n]...)
			r.pendingErr = err
			return one[0], nil
		}
		if err != nil {
			return 0, err
		}
	}
}

func (r *jsonStreamReader) drainJQLiteral(offset int64) error {
	i, ok := r.index(offset)
	if !ok {
		return errors.New("invalid JSON literal offset")
	}
	for _, c := range r.delivered[i:] {
		if !isJQLiteralByte(c) {
			return nil
		}
	}
	buf := make([]byte, inputReadChunk)
	for {
		n, err := r.Read(buf)
		for _, c := range buf[:n] {
			if !isJQLiteralByte(c) {
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *jsonStreamReader) drainJSONString(escaped bool) error {
	buf := make([]byte, inputReadChunk)
	for {
		n, err := r.Read(buf)
		for _, c := range buf[:n] {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *jsonStreamReader) drainNextJQToken(ctx context.Context) error {
	buf := make([]byte, inputReadChunk)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(buf)
		for i, c := range buf[:n] {
			if isJSONWhitespace(c) {
				continue
			}
			readErr := err
			if i+1 < n {
				r.prependPending(buf[i+1:n], err)
				err = nil
			}
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}
			switch {
			case c == '"':
				return r.drainJSONString(false)
			case isJQLiteralByte(c):
				return r.drainJQLiteral(r.base + int64(len(r.delivered)) - 1)
			default:
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *jsonStreamReader) prependPending(data []byte, err error) {
	r.delivered = r.delivered[:len(r.delivered)-len(data)]
	pending := make([]byte, 0, len(data)+len(r.pending))
	pending = append(pending, data...)
	r.pending = append(pending, r.pending...)
	if err != nil {
		r.pendingErr = err
	}
}

func isJQLiteralByte(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '"', '[', ',', ']', '{', ':', '}':
		return false
	default:
		return true
	}
}

func isPrimitiveBoundary(next byte) bool {
	switch next {
	case ' ', '\t', '\r', '\n', '{', '[', '"':
		return true
	default:
		return false
	}
}

func (d *jsonValueDecoder) decode(depth int) (value, error) {
	if err := d.ctx.Err(); err != nil {
		return value{}, err
	}
	token, err := d.dec.Token()
	if err != nil {
		return value{}, err
	}
	switch token := token.(type) {
	case nil:
		return null(), nil
	case bool:
		return boolean(token), nil
	case string:
		return stringValue(token)
	case json.Number:
		return parseNumber(string(token))
	case json.Delim:
		if depth >= MaxNestingDepth && (token == '[' || token == '{') {
			return value{}, errValueDepth
		}
		switch token {
		case '[':
			items := make([]value, 0)
			nodes, size := 1, 2
			for d.dec.More() {
				if err := d.ctx.Err(); err != nil {
					return value{}, err
				}
				item, err := d.decode(depth + 1)
				if err != nil {
					if errors.Is(err, io.EOF) {
						return value{}, io.ErrUnexpectedEOF
					}
					return value{}, err
				}
				separator := 0
				if len(items) > 0 {
					separator = 1
				}
				if err := addAggregate(&nodes, &size, item, separator, MaxValueNodes, MaxValueBytes); err != nil {
					return value{}, err
				}
				items = append(items, item)
			}
			end, err := d.dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return value{}, io.ErrUnexpectedEOF
				}
				return value{}, err
			}
			if end != json.Delim(']') {
				return value{}, errors.New("invalid JSON array")
			}
			return arrayValue(items)
		case '{':
			items := make([]objectMember, 0)
			nodes, size := 1, 2
			for d.dec.More() {
				if err := d.ctx.Err(); err != nil {
					return value{}, err
				}
				keyToken, err := d.dec.Token()
				if err != nil {
					if errors.Is(err, io.EOF) {
						return value{}, io.ErrUnexpectedEOF
					}
					return value{}, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return value{}, errors.New("invalid JSON object key")
				}
				item, err := d.decode(depth + 1)
				if err != nil {
					if errors.Is(err, io.EOF) {
						return value{}, io.ErrUnexpectedEOF
					}
					return value{}, err
				}
				separator := 0
				if len(items) > 0 {
					separator = 1
				}
				if err := addAggregate(&nodes, &size, item, len(key)+3+separator, MaxValueNodes, MaxValueBytes); err != nil {
					return value{}, err
				}
				items = append(items, objectMember{key: key, value: item})
			}
			end, err := d.dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return value{}, io.ErrUnexpectedEOF
				}
				return value{}, err
			}
			if end != json.Delim('}') {
				return value{}, errors.New("invalid JSON object")
			}
			return object(items)
		default:
			return value{}, errors.New("unexpected closing delimiter")
		}
	default:
		return value{}, errors.New("unsupported JSON token")
	}
}

func parseSingleJSON(ctx context.Context, text string) (value, error) {
	if len(text) > MaxValueBytes {
		return value{}, errValueBytes
	}
	if err := validateSurrogates(text); err != nil {
		return value{}, err
	}
	decoder := newJSONValueDecoder(ctx, strings.NewReader(text))
	v, err := decoder.next()
	if err != nil {
		return value{}, err
	}
	_, err = decoder.next()
	if !errors.Is(err, io.EOF) {
		if err == nil {
			return value{}, errors.New("expected exactly one JSON value")
		}
		return value{}, err
	}
	return v, nil
}

type boundedBuilder struct {
	b   strings.Builder
	max int
}

func (b *boundedBuilder) writeString(s string) error {
	if len(s) > b.max-b.b.Len() {
		return errOutputLimit
	}
	_, _ = b.b.WriteString(s)
	return nil
}

func (b *boundedBuilder) writeByte(c byte) error {
	if b.b.Len() >= b.max {
		return errOutputLimit
	}
	_ = b.b.WriteByte(c)
	return nil
}

func encodeValue(v value, pretty bool, limit int) (string, error) {
	b := &boundedBuilder{max: limit}
	if err := appendValue(b, v, pretty, 0); err != nil {
		return "", err
	}
	return b.b.String(), nil
}

func appendValue(b *boundedBuilder, v value, pretty bool, depth int) error {
	switch v.kind {
	case valueNull:
		return b.writeString("null")
	case valueBoolean:
		if v.bool {
			return b.writeString("true")
		}
		return b.writeString("false")
	case valueNumber:
		return b.writeString(v.num.text())
	case valueString:
		return appendJSONString(b, v.str)
	case valueArray:
		if err := b.writeByte('['); err != nil {
			return err
		}
		for i := range v.array {
			if i > 0 {
				if err := b.writeByte(','); err != nil {
					return err
				}
			}
			if pretty {
				if err := b.writeByte('\n'); err != nil {
					return err
				}
				if err := appendIndent(b, depth+1); err != nil {
					return err
				}
			}
			if err := appendValue(b, v.array[i], pretty, depth+1); err != nil {
				return err
			}
		}
		if pretty && len(v.array) > 0 {
			if err := b.writeByte('\n'); err != nil {
				return err
			}
			if err := appendIndent(b, depth); err != nil {
				return err
			}
		}
		return b.writeByte(']')
	case valueObject:
		if err := b.writeByte('{'); err != nil {
			return err
		}
		for i := range v.obj.members {
			if i > 0 {
				if err := b.writeByte(','); err != nil {
					return err
				}
			}
			if pretty {
				if err := b.writeByte('\n'); err != nil {
					return err
				}
				if err := appendIndent(b, depth+1); err != nil {
					return err
				}
			}
			member := &v.obj.members[i]
			if err := appendJSONString(b, member.key); err != nil {
				return err
			}
			if pretty {
				if err := b.writeString(": "); err != nil {
					return err
				}
			} else if err := b.writeByte(':'); err != nil {
				return err
			}
			if err := appendValue(b, member.value, pretty, depth+1); err != nil {
				return err
			}
		}
		if pretty && len(v.obj.members) > 0 {
			if err := b.writeByte('\n'); err != nil {
				return err
			}
			if err := appendIndent(b, depth); err != nil {
				return err
			}
		}
		return b.writeByte('}')
	default:
		return errors.New("invalid value")
	}
}

func appendIndent(b *boundedBuilder, depth int) error {
	for range depth * 2 {
		if err := b.writeByte(' '); err != nil {
			return err
		}
	}
	return nil
}

func appendJSONString(b *boundedBuilder, s string) error {
	if err := b.writeByte('"'); err != nil {
		return err
	}
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		switch r {
		case '"':
			if err := b.writeString(`\"`); err != nil {
				return err
			}
		case '\\':
			if err := b.writeString(`\\`); err != nil {
				return err
			}
		case '\b':
			if err := b.writeString(`\b`); err != nil {
				return err
			}
		case '\f':
			if err := b.writeString(`\f`); err != nil {
				return err
			}
		case '\n':
			if err := b.writeString(`\n`); err != nil {
				return err
			}
		case '\r':
			if err := b.writeString(`\r`); err != nil {
				return err
			}
		case '\t':
			if err := b.writeString(`\t`); err != nil {
				return err
			}
		default:
			if r < 0x20 || r == 0x7f {
				if err := b.writeString(fmt.Sprintf(`\u%04x`, r)); err != nil {
					return err
				}
			} else if err := b.writeString(string(r)); err != nil {
				return err
			}
		}
	}
	return b.writeByte('"')
}
