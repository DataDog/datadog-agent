// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"bytes"
	"fmt"
	"strings"
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
	rd       *strings.Reader // the "rune" reader
	pos      int             // byte offset of lastChar
	lastChar rune            // last read rune
	err      error           // any error occurred while reading

	literalEscapes bool // indicates we should not treat backslashes as escape characters
	seenEscape     bool // indicates whether this tokenizer has seen an escape character within a string
}

// NewSQLTokenizer creates a new SQLTokenizer for the given SQL string. The literalEscapes argument specifies
// whether escape characters should be treated literally or as such.
func NewSQLTokenizer(sql string, literalEscapes bool) *SQLTokenizer {
	return &SQLTokenizer{
		rd:             strings.NewReader(sql),
		literalEscapes: literalEscapes,
	}
}

// Reset the underlying buffer and positions
func (tkn *SQLTokenizer) Reset(in string) {
	tkn.rd.Reset(in)
	tkn.pos = 0
	tkn.lastChar = 0
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
		tkn.next()
	}
	tkn.skipBlank()

	switch ch := tkn.lastChar; {
	case isLeadingLetter(ch):
		return tkn.scanIdentifier()
	case isDigit(ch):
		return tkn.scanNumber(false)
	default:
		tkn.next()
		switch ch {
		case EOFChar:
			return EOFChar, nil
		case ':':
			if tkn.lastChar == ':' {
				tkn.next()
				return ColonCast, []byte("::")
			}
			if tkn.lastChar != '=' {
				return tkn.scanBindVar()
			}
			fallthrough
		case '=', ',', ';', '(', ')', '+', '*', '&', '|', '^', '~', '[', ']', '?':
			return TokenKind(ch), runeBytes(ch)
		case '.':
			if isDigit(tkn.lastChar) {
				return tkn.scanNumber(true)
			}
			return TokenKind(ch), runeBytes(ch)
		case '/':
			switch tkn.lastChar {
			case '/':
				tkn.next()
				return tkn.scanCommentType1("//")
			case '*':
				tkn.next()
				return tkn.scanCommentType2()
			default:
				return TokenKind(ch), runeBytes(ch)
			}
		case '-':
			if tkn.lastChar == '-' {
				tkn.next()
				return tkn.scanCommentType1("--")
			}
			return TokenKind(ch), runeBytes(ch)
		case '#':
			tkn.next()
			return tkn.scanCommentType1("#")
		case '<':
			switch tkn.lastChar {
			case '>':
				tkn.next()
				return NE, []byte("<>")
			case '=':
				tkn.next()
				switch tkn.lastChar {
				case '>':
					tkn.next()
					return NullSafeEqual, []byte("<=>")
				default:
					return LE, []byte("<=")
				}
			default:
				return TokenKind(ch), runeBytes(ch)
			}
		case '>':
			if tkn.lastChar == '=' {
				tkn.next()
				return GE, []byte(">=")
			}
			return TokenKind(ch), runeBytes(ch)
		case '!':
			if tkn.lastChar == '=' {
				tkn.next()
				return NE, []byte("!=")
			}
			tkn.setErr(`expected "=" after "!", got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
			return LexError, []byte("!")
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
			return TokenKind(ch), runeBytes(ch)
		case '$':
			return tkn.scanPreparedStatement('$')
		case '{':
			return tkn.scanEscapeSequence('{')
		default:
			tkn.setErr(`unexpected byte %d`, ch)
			return LexError, runeBytes(ch)
		}
	}
}

func (tkn *SQLTokenizer) skipBlank() {
	for unicode.IsSpace(tkn.lastChar) {
		tkn.next()
	}
}

func (tkn *SQLTokenizer) scanIdentifier() (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(tkn.lastChar)
	tkn.next()

	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' || tkn.lastChar == '*' {
		buffer.WriteRune(tkn.lastChar)
		tkn.next()
	}
	upper := bytes.ToUpper(buffer.Bytes())
	if keywordID, found := keywords[string(upper)]; found {
		return keywordID, buffer.Bytes()
	}
	return ID, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanLiteralIdentifier(quote rune) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(tkn.lastChar)
	if !isLetter(tkn.lastChar) && !isDigit(tkn.lastChar) {
		tkn.setErr(`unexpected character "%c" (%d) in literal identifier`, tkn.lastChar, tkn.lastChar)
		return LexError, buffer.Bytes()
	}
	for tkn.next(); skipNonLiteralIdentifier(tkn.lastChar); tkn.next() {
		buffer.WriteRune(tkn.lastChar)
	}
	// literals identifier are enclosed between quotes
	if tkn.lastChar != quote {
		tkn.setErr(`literal identifiers must end in "%c", got "%c" (%d)`, quote, tkn.lastChar, tkn.lastChar)
		return LexError, buffer.Bytes()
	}
	tkn.next()
	return ID, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanVariableIdentifier(prefix rune) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(prefix)
	buffer.WriteRune(tkn.lastChar)

	for tkn.next(); tkn.lastChar != ')' && tkn.lastChar != EOFChar; tkn.next() {
		buffer.WriteRune(tkn.lastChar)
	}

	buffer.WriteRune(tkn.lastChar)
	tkn.next()
	buffer.WriteRune(tkn.lastChar)
	if !isLetter(tkn.lastChar) {
		tkn.setErr(`invalid character after variable identifier: "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, buffer.Bytes()
	}
	tkn.next()
	return Variable, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanFormatParameter(prefix rune) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(prefix)
	buffer.WriteRune(tkn.lastChar)

	tkn.next()
	return Variable, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanPreparedStatement(prefix rune) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}

	// a prepared statement expect a digit identifier like $1
	if !isDigit(tkn.lastChar) {
		tkn.setErr(`prepared statements must start with digits, got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, buffer.Bytes()
	}

	// read numbers and return an error if any
	token, buff := tkn.scanNumber(false)
	if token == LexError {
		tkn.setErr("invalid number")
		return LexError, buffer.Bytes()
	}

	buffer.WriteRune(prefix)
	buffer.Write(buff)
	return PreparedStatement, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanEscapeSequence(braces rune) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(braces)

	for tkn.lastChar != '}' && tkn.lastChar != EOFChar {
		buffer.WriteRune(tkn.lastChar)
		tkn.next()
	}

	// we've reached the end of the string without finding
	// the closing curly braces
	if tkn.lastChar == EOFChar {
		tkn.setErr("unexpected EOF in escape sequence")
		return LexError, buffer.Bytes()
	}

	buffer.WriteRune(tkn.lastChar)
	tkn.next()
	return EscapeSequence, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanBindVar() (TokenKind, []byte) {
	buffer := bytes.NewBufferString(":")
	token := ValueArg
	if tkn.lastChar == ':' {
		token = ListArg
		buffer.WriteRune(tkn.lastChar)
		tkn.next()
	}
	if !isLetter(tkn.lastChar) {
		tkn.setErr(`bind variables should start with letters, got "%c" (%d)`, tkn.lastChar, tkn.lastChar)
		return LexError, buffer.Bytes()
	}
	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' {
		buffer.WriteRune(tkn.lastChar)
		tkn.next()
	}
	return token, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanMantissa(base int, buffer *bytes.Buffer) {
	for digitVal(tkn.lastChar) < base {
		tkn.consumeNext(buffer)
	}
}

func (tkn *SQLTokenizer) scanNumber(seenDecimalPoint bool) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	if seenDecimalPoint {
		buffer.WriteByte('.')
		tkn.scanMantissa(10, buffer)
		goto exponent
	}

	if tkn.lastChar == '0' {
		// int or float
		tkn.consumeNext(buffer)
		if tkn.lastChar == 'x' || tkn.lastChar == 'X' {
			// hexadecimal int
			tkn.consumeNext(buffer)
			tkn.scanMantissa(16, buffer)
		} else {
			// octal int or float
			seenDecimalDigit := false
			tkn.scanMantissa(8, buffer)
			if tkn.lastChar == '8' || tkn.lastChar == '9' {
				// illegal octal int or float
				seenDecimalDigit = true
				tkn.scanMantissa(10, buffer)
			}
			if tkn.lastChar == '.' || tkn.lastChar == 'e' || tkn.lastChar == 'E' {
				goto fraction
			}
			// octal int
			if seenDecimalDigit {
				// tkn.setErr called in caller
				return LexError, buffer.Bytes()
			}
		}
		goto exit
	}

	// decimal int or float
	tkn.scanMantissa(10, buffer)

fraction:
	if tkn.lastChar == '.' {
		tkn.consumeNext(buffer)
		tkn.scanMantissa(10, buffer)
	}

exponent:
	if tkn.lastChar == 'e' || tkn.lastChar == 'E' {
		tkn.consumeNext(buffer)
		if tkn.lastChar == '+' || tkn.lastChar == '-' {
			tkn.consumeNext(buffer)
		}
		tkn.scanMantissa(10, buffer)
	}

exit:
	if buffer.Len() == 0 {
		return LexError, nil
	}
	return Number, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanString(delim rune, kind TokenKind) (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	for {
		ch := tkn.lastChar
		tkn.next()
		if ch == delim {
			if tkn.lastChar == delim {
				// doubling a delimiter is the default way to embed the delimiter within a string
				tkn.next()
			} else {
				// a single delimiter denotes the end of the string
				break
			}
		} else if ch == escapeCharacter {
			tkn.seenEscape = true

			if !tkn.literalEscapes {
				// treat as an escape character
				ch = tkn.lastChar
				tkn.next()
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
	buffer := &bytes.Buffer{}
	buffer.WriteString(prefix)
	for tkn.lastChar != EOFChar {
		if tkn.lastChar == '\n' {
			tkn.consumeNext(buffer)
			break
		}
		tkn.consumeNext(buffer)
	}
	return Comment, buffer.Bytes()
}

func (tkn *SQLTokenizer) scanCommentType2() (TokenKind, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteString("/*")
	for {
		if tkn.lastChar == '*' {
			tkn.consumeNext(buffer)
			if tkn.lastChar == '/' {
				tkn.consumeNext(buffer)
				break
			}
			continue
		}
		if tkn.lastChar == EOFChar {
			tkn.setErr("unexpected EOF in comment")
			return LexError, buffer.Bytes()
		}
		tkn.consumeNext(buffer)
	}
	return Comment, buffer.Bytes()
}

func (tkn *SQLTokenizer) consumeNext(buffer *bytes.Buffer) {
	if tkn.lastChar == EOFChar {
		// This should never happen.
		panic("unexpected EOF")
	}
	buffer.WriteRune(tkn.lastChar)
	tkn.next()
}

func (tkn *SQLTokenizer) next() {
	ch, _, err := tkn.rd.ReadRune()
	if tkn.lastChar != 0 || tkn.pos > 0 {
		// we are past the first character
		tkn.pos += utf8.RuneLen(ch)
	}
	if err != nil {
		// only EOF is possible
		tkn.lastChar = EOFChar
	} else {
		tkn.lastChar = ch
	}
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
