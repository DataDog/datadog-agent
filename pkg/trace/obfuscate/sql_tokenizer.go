// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"bytes"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// tokenizer.go implemenents a lexer-like iterator that tokenizes SQL and CQL
// strings, so that an external component can filter or alter each token of the
// string. This implementation can't be used as a real SQL lexer (so a parser
// cannot build the AST) because many rules are ignored to make the tokenizer
// simpler.
// This implementation was inspired by https://github.com/youtube/vitess sql parser
// TODO: add the license to the NOTICE file

// TokenKind specifies the type of the token being scanned. It may be one of the defined
// constants below or in some cases the actual rune itself.
type TokenKind uint32

// EOFChar is used to signal that no more characters were found by the scanner. It is
// an invalid rune value that can not be found in any valid string.
const EOFChar = unicode.MaxRune + 1

// list of available tokens; this list has been reduced because we don't
// need a full-fledged tokenizer to implement a Lexer
const (
	LexError = TokenKind(57346) + iota

	ID
	Limit
	Null
	String
	DoubleQuotedString
	Number
	BooleanLiteral
	ValueArg
	ListArg
	Comment
	Variable
	Savepoint
	PreparedStatement
	EscapeSequence
	NullSafeEqual
	LE
	GE
	NE
	As
	From
	Update
	Insert
	Into
	Join
	ColonCast

	// FilteredGroupable specifies that the given token has been discarded by one of the
	// token filters and that it is groupable together with consecutive FilteredGroupable
	// tokens.
	FilteredGroupable

	// Filtered specifies that the token is a comma and was discarded by one
	// of the filters.
	Filtered

	// FilteredBracketedIdentifier specifies that we are currently discarding
	// a bracketed identifier (MSSQL).
	// See issue https://github.com/DataDog/datadog-trace-agent/issues/475.
	FilteredBracketedIdentifier
)

const escapeCharacter = '\\'

// SQLTokenizer is the struct used to generate SQL
// tokens for the parser.
type SQLTokenizer struct {
	pos      int    // byte offset of lastChar
	lastChar rune   // last read rune
	buf      []byte // buf holds the query that we are parsing
	off      int    // off is the index into buf where the unread portion of the query begins.
	err      error  // any error occurred while reading

	literalEscapes bool // indicates we should not treat backslashes as escape characters
	seenEscape     bool // indicates whether this tokenizer has seen an escape character within a string
}

// NewSQLTokenizer creates a new SQLTokenizer for the given SQL string. The literalEscapes argument specifies
// whether escape characters should be treated literally or as such.
func NewSQLTokenizer(sql string, literalEscapes bool) *SQLTokenizer {
	return &SQLTokenizer{
		buf:            []byte(sql),
		literalEscapes: literalEscapes,
	}
}

// Reset the underlying buffer and positions
func (tkn *SQLTokenizer) Reset(in string) {
	tkn.pos = 0
	tkn.lastChar = 0
	tkn.buf = []byte(in)
	tkn.off = 0
	tkn.err = nil
}

// keywords used to recognize string tokens
var keywords = map[string]TokenKind{
	"NULL":      Null,
	"TRUE":      BooleanLiteral,
	"FALSE":     BooleanLiteral,
	"SAVEPOINT": Savepoint,
	"LIMIT":     Limit,
	"AS":        As,
	"FROM":      From,
	"UPDATE":    Update,
	"INSERT":    Insert,
	"INTO":      Into,
	"JOIN":      Join,
}

// Err returns the last error that the tokenizer encountered, or nil.
func (tkn *SQLTokenizer) Err() error { return tkn.err }

func (tkn *SQLTokenizer) setErr(format string, args ...interface{}) {
	tkn.err = fmt.Errorf("at position %d: %v", tkn.pos, fmt.Errorf(format, args...))
}

// SeenEscape returns whether or not this tokenizer has seen an escape character within a scanned string
func (tkn *SQLTokenizer) SeenEscape() bool { return tkn.seenEscape }

// Scan scans the tokenizer for the next token and returns
// the token type and the token buffer.
func (tkn *SQLTokenizer) Scan() (TokenKind, []byte) {
	if tkn.lastChar == 0 {
		tkn.advance()
	}
	tkn.skipBlank()

	switch ch := tkn.lastChar; {
	case isLeadingLetter(ch):
		return tkn.scanIdentifier()
	case isDigit(ch):
		return tkn.scanNumber(false)
	default:
		tkn.advance()
		switch ch {
		case EOFChar:
			return EOFChar, nil
		case ':':
			if tkn.lastChar == ':' {
				tkn.advance()
				return ColonCast, []byte("::")
			}
			if tkn.lastChar != '=' {
				return tkn.scanBindVar()
			}
			fallthrough
		case '=', ',', ';', '(', ')', '+', '*', '&', '|', '^', '~', '[', ']', '?':
			return TokenKind(ch), tkn.bytes()
		case '.':
			if isDigit(tkn.lastChar) {
				return tkn.scanNumber(true)
			}
			return TokenKind(ch), tkn.bytes()
		case '/':
			switch tkn.lastChar {
			case '/':
				tkn.advance()
				return tkn.scanCommentType1("//")
			case '*':
				tkn.advance()
				return tkn.scanCommentType2()
			default:
				return TokenKind(ch), tkn.bytes()
			}
		case '-':
			if tkn.lastChar == '-' {
				tkn.advance()
				return tkn.scanCommentType1("--")
			}
			return TokenKind(ch), tkn.bytes()
		case '#':
			tkn.advance()
			return tkn.scanCommentType1("#")
		case '<':
			switch tkn.lastChar {
			case '>':
				tkn.advance()
				return NE, []byte("<>")
			case '=':
				tkn.advance()
				switch tkn.lastChar {
				case '>':
					tkn.advance()
					return NullSafeEqual, []byte("<=>")
				default:
					return LE, []byte("<=")
				}
			default:
				return TokenKind(ch), tkn.bytes()
			}
		case '>':
			if tkn.lastChar == '=' {
				tkn.advance()
				return GE, []byte(">=")
			}
			return TokenKind(ch), tkn.bytes()
		case '!':
			if tkn.lastChar == '=' {
				tkn.advance()
				return NE, []byte("!=")
			}
			tkn.setErr(`expected "=" after "!", got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
			return LexError, tkn.bytes()
		case '\'':
			return tkn.scanString(ch, String)
		case '"':
			return tkn.scanString(ch, DoubleQuotedString)
		case '`':
			return tkn.scanLiteralIdentifier('`')
		case '%':
			if tkn.lastChar == '(' {
				return tkn.scanVariableIdentifier('%')
			}
			if isLetter(tkn.lastChar) {
				// format parameter (e.g. '%s')
				return tkn.scanFormatParameter('%')
			}
			// modulo operator (e.g. 'id % 8')
			return TokenKind(ch), tkn.bytes()
		case '$':
			return tkn.scanPreparedStatement('$')
		case '{':
			return tkn.scanEscapeSequence('{')
		default:
			tkn.setErr(`unexpected byte %d`, ch)
			return LexError, tkn.bytes()
		}
	}
}

func (tkn *SQLTokenizer) skipBlank() {
	for unicode.IsSpace(tkn.lastChar) {
		tkn.advance()
	}
	tkn.bytes()
}

// toUpper is a modified version of bytes.ToUpper. It returns an upper-cased version of the byte
// slice src with all Unicode letters mapped to their upper case. It is modified to also accept a
// byte slice dst as an argument, the underlying storage of which (up to the capacity of dst)
// will be used as the destination of the upper-case copy of src, if it fits. As a special case,
// toUpper will return src if the byte slice is already upper-case. This function is used rather
// than bytes.ToUpper to improve the memory performance of the obfuscator by saving unnecessary
// allocations happening in bytes.ToUpper
func toUpper(src, dst []byte) []byte {
	dst = dst[:0]
	isASCII, hasLower := true, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		hasLower = hasLower || ('a' <= c && c <= 'z')
	}
	if cap(dst) < len(src) {
		dst = make([]byte, 0, len(src))
	}
	if isASCII { // optimize for ASCII-only byte slices.
		if !hasLower {
			// Just return src.
			return src
		}
		dst = dst[:len(src)]
		for i := 0; i < len(src); i++ {
			c := src[i]
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			dst[i] = c
		}
		return dst
	}
	// This *could* be optimized, but it's an uncommon case.
	return bytes.Map(unicode.ToUpper, src)
}

func (tkn *SQLTokenizer) scanIdentifier() (TokenKind, []byte) {
	tkn.advance()
	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' || tkn.lastChar == '*' {
		tkn.advance()
	}

	t := tkn.bytes()
	// Space allows us to upper-case identifiers 256 bytes long or less without allocating heap
	// storage for them, since space is allocated on the stack. A size of 256 bytes was chosen
	// based on the allowed length of sql identifiers in various sql implementations.
	var space [256]byte
	upper := toUpper(t, space[:0])
	if keywordID, found := keywords[string(upper)]; found {
		return keywordID, t
	}
	return ID, t
}

func (tkn *SQLTokenizer) scanLiteralIdentifier(quote rune) (TokenKind, []byte) {
	tkn.bytes() // throw away initial quote
	if !isLetter(tkn.lastChar) && !isDigit(tkn.lastChar) {
		tkn.setErr(`unexpected character "%c" (%d) in literal identifier`, tkn.lastChar, tkn.lastChar)
		return LexError, tkn.bytes()
	}
	for tkn.advance(); skipNonLiteralIdentifier(tkn.lastChar); tkn.advance() {
	}

	t := tkn.bytes()
	// literals identifier are enclosed between quotes
	if tkn.lastChar != quote {
		tkn.setErr(`literal identifiers must end in "%c", got "%c" (%d)`, quote, tkn.lastChar, tkn.lastChar)
		return LexError, t
	}
	tkn.advance()
	return ID, t
}

func (tkn *SQLTokenizer) scanVariableIdentifier(prefix rune) (TokenKind, []byte) {
	for tkn.advance(); tkn.lastChar != ')' && tkn.lastChar != EOFChar; tkn.advance() {
	}
	tkn.advance()
	if !isLetter(tkn.lastChar) {
		tkn.setErr(`invalid character after variable identifier: "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, tkn.bytes()
	}
	tkn.advance()
	return Variable, tkn.bytes()
}

func (tkn *SQLTokenizer) scanFormatParameter(prefix rune) (TokenKind, []byte) {
	tkn.advance()
	return Variable, tkn.bytes()
}

func (tkn *SQLTokenizer) scanPreparedStatement(prefix rune) (TokenKind, []byte) {
	// a prepared statement expect a digit identifier like $1
	if !isDigit(tkn.lastChar) {
		tkn.setErr(`prepared statements must start with digits, got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, tkn.bytes()
	}

	// scanNumber keeps the prefix rune intact.
	// read numbers and return an error if any
	token, buff := tkn.scanNumber(false)
	if token == LexError {
		tkn.setErr("invalid number")
		return LexError, tkn.bytes()
	}
	return PreparedStatement, buff
}

func (tkn *SQLTokenizer) scanEscapeSequence(braces rune) (TokenKind, []byte) {
	for tkn.lastChar != '}' && tkn.lastChar != EOFChar {
		tkn.advance()
	}

	// we've reached the end of the string without finding
	// the closing curly braces
	if tkn.lastChar == EOFChar {
		tkn.setErr("unexpected EOF in escape sequence")
		return LexError, tkn.bytes()
	}

	tkn.advance()
	return EscapeSequence, tkn.bytes()
}

func (tkn *SQLTokenizer) scanBindVar() (TokenKind, []byte) {
	token := ValueArg
	if tkn.lastChar == ':' {
		token = ListArg
		tkn.advance()
	}
	if !isLetter(tkn.lastChar) {
		tkn.setErr(`bind variables should start with letters, got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, tkn.bytes()
	}
	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' {
		tkn.advance()
	}
	return token, tkn.bytes()
}

func (tkn *SQLTokenizer) scanMantissa(base int) {
	for digitVal(tkn.lastChar) < base {
		tkn.advance()
	}
}

func (tkn *SQLTokenizer) scanNumber(seenDecimalPoint bool) (TokenKind, []byte) {
	if seenDecimalPoint {
		tkn.scanMantissa(10)
		goto exponent
	}

	if tkn.lastChar == '0' {
		// int or float
		tkn.advance()
		if tkn.lastChar == 'x' || tkn.lastChar == 'X' {
			// hexadecimal int
			tkn.advance()
			tkn.scanMantissa(16)
		} else {
			// octal int or float
			seenDecimalDigit := false
			tkn.scanMantissa(8)
			if tkn.lastChar == '8' || tkn.lastChar == '9' {
				// illegal octal int or float
				seenDecimalDigit = true
				tkn.scanMantissa(10)
			}
			if tkn.lastChar == '.' || tkn.lastChar == 'e' || tkn.lastChar == 'E' {
				goto fraction
			}
			// octal int
			if seenDecimalDigit {
				// tkn.setErr called in caller
				return LexError, tkn.bytes()
			}
		}
		goto exit
	}

	// decimal int or float
	tkn.scanMantissa(10)

fraction:
	if tkn.lastChar == '.' {
		tkn.advance()
		tkn.scanMantissa(10)
	}

exponent:
	if tkn.lastChar == 'e' || tkn.lastChar == 'E' {
		tkn.advance()
		if tkn.lastChar == '+' || tkn.lastChar == '-' {
			tkn.advance()
		}
		tkn.scanMantissa(10)
	}

exit:
	t := tkn.bytes()
	if len(t) == 0 {
		return LexError, nil
	}
	return Number, t
}

func (tkn *SQLTokenizer) scanString(delim rune, kind TokenKind) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	for {
		ch := tkn.lastChar
		tkn.advance()
		if ch == delim {
			if tkn.lastChar == delim {
				// doubling a delimiter is the default way to embed the delimiter within a string
				tkn.advance()
			} else {
				// a single delimiter denotes the end of the string
				break
			}
		} else if ch == escapeCharacter {
			tkn.seenEscape = true

			if !tkn.literalEscapes {
				// treat as an escape character
				ch = tkn.lastChar
				tkn.advance()
			}
		}
		if ch == EOFChar {
			tkn.setErr("unexpected EOF in string")
			return LexError, buffer.Bytes()
		}
		buffer.WriteRune(ch)
	}
	buf := buffer.Bytes()
	if kind == ID && len(buf) == 0 || bytes.IndexFunc(buf, func(r rune) bool { return !unicode.IsSpace(r) }) == -1 {
		// This string is an empty or white-space only identifier.
		// We should keep the start and end delimiters in order to
		// avoid creating invalid queries.
		// See: https://github.com/DataDog/datadog-trace-agent/issues/316
		return kind, append(runeBytes(delim), runeBytes(delim)...)
	}
	return kind, buf
}

func (tkn *SQLTokenizer) scanCommentType1(prefix string) (TokenKind, []byte) {
	for tkn.lastChar != EOFChar {
		if tkn.lastChar == '\n' {
			tkn.advance()
			break
		}
		tkn.advance()
	}
	return Comment, tkn.bytes()
}

func (tkn *SQLTokenizer) scanCommentType2() (TokenKind, []byte) {
	for {
		if tkn.lastChar == '*' {
			tkn.advance()
			if tkn.lastChar == '/' {
				tkn.advance()
				break
			}
			continue
		}
		if tkn.lastChar == EOFChar {
			tkn.setErr("unexpected EOF in comment")
			return LexError, tkn.bytes()
		}
		tkn.advance()
	}
	return Comment, tkn.bytes()
}

// advance advances the tokenizer to the next rune. If this is not possible, tkn.lastChar will
// be set to EOFChar.
func (tkn *SQLTokenizer) advance() {
	ch, n := utf8.DecodeRune(tkn.buf[tkn.off:])
	if ch == utf8.RuneError && n == 0 {
		// only EOF is possible
		tkn.pos++
		tkn.lastChar = EOFChar
		return
	}

	len := utf8.RuneLen(ch)
	if tkn.lastChar != 0 || tkn.pos > 0 {
		// we are past the first character
		tkn.pos += len
	}
	tkn.off += len
	tkn.lastChar = ch
}

// bytes returns all the bytes that were advanced over since its last call.
func (tkn *SQLTokenizer) bytes() []byte {
	if tkn.lastChar == EOFChar {
		ret := tkn.buf[:tkn.off]
		tkn.buf = tkn.buf[tkn.off:]
		tkn.off = 0
		return ret
	}
	ret := tkn.buf[:tkn.off-1]
	tkn.buf = tkn.buf[tkn.off-1:]
	tkn.off = 1
	return ret
}

func skipNonLiteralIdentifier(ch rune) bool {
	return isLetter(ch) || isDigit(ch) || '.' == ch || '-' == ch
}

func isLeadingLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == '@'
}

func isLetter(ch rune) bool {
	return isLeadingLetter(ch) || ch == '#'
}

func digitVal(ch rune) int {
	switch {
	case '0' <= ch && ch <= '9':
		return int(ch) - '0'
	case 'a' <= ch && ch <= 'f':
		return int(ch) - 'a' + 10
	case 'A' <= ch && ch <= 'F':
		return int(ch) - 'A' + 10
	}
	return 16 // larger than any legal digit val
}

func isDigit(ch rune) bool { return '0' <= ch && ch <= '9' }

// runeBytes converts the given rune to a slice of bytes.
func runeBytes(r rune) []byte {
	buf := make([]byte, utf8.UTFMax)
	n := utf8.EncodeRune(buf, r)
	return buf[:n]
}
