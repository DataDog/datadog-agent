// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package obfuscate

import (
	"bytes"
	"strings"
	"unicode"
)

// tokenizer.go implemenents a lexer-like iterator that tokenizes SQL and CQL
// strings, so that an external component can filter or alter each token of the
// string. This implementation can't be used as a real SQL lexer (so a parser
// cannot build the AST) because many rules are ignored to make the tokenizer
// simpler.
// This implementation was inspired by https://github.com/youtube/vitess sql parser
// TODO: add the license to the NOTICE file

// list of available tokens; this list has been reduced because we don't
// need a full-fledged tokenizer to implement a Lexer
const (
	EOFChar           = 0x100
	LexError          = 57346
	ID                = 57347
	Limit             = 57348
	Null              = 57349
	String            = 57350
	Number            = 57351
	BooleanLiteral    = 57352
	ValueArg          = 57353
	ListArg           = 57354
	Comment           = 57355
	Variable          = 57356
	Savepoint         = 57357
	PreparedStatement = 57358
	EscapeSequence    = 57359
	NullSafeEqual     = 57360
	LE                = 57361
	GE                = 57362
	NE                = 57363
	As                = 57365

	// Filtered specifies that the given token has been discarded by one of the
	// token filters.
	Filtered = 57364

	// FilteredComma specifies that the token is a comma and was discarded by one
	// of the filters.
	FilteredComma = 57366

	// FilteredBracketedIdentifier specifies that we are currently discarding
	// a bracketed identifier (MSSQL).
	// See issue https://github.com/DataDog/datadog-trace-agent/issues/475.
	FilteredBracketedIdentifier = 57367
)

// Tokenizer is the struct used to generate SQL
// tokens for the parser.
type Tokenizer struct {
	InStream *strings.Reader
	Position int
	lastChar uint16
}

// NewStringTokenizer creates a new Tokenizer for the
// sql string.
func NewStringTokenizer(sql string) *Tokenizer {
	return &Tokenizer{InStream: strings.NewReader(sql)}
}

// Reset the underlying buffer and positions
func (tkn *Tokenizer) Reset(in string) {
	tkn.InStream.Reset(in)
	tkn.Position = 0
	tkn.lastChar = 0
}

// keywords used to recognize string tokens
var keywords = map[string]int{
	"NULL":      Null,
	"TRUE":      BooleanLiteral,
	"FALSE":     BooleanLiteral,
	"SAVEPOINT": Savepoint,
	"LIMIT":     Limit,
	"AS":        As,
}

// Scan scans the tokenizer for the next token and returns
// the token type and the token buffer.
// TODO[manu]: the current implementation returns a new Buffer
// for each Scan(). An improvement to reduce the overhead of
// the Scan() is to return slices instead of buffers.
func (tkn *Tokenizer) Scan() (int, []byte) {
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
			if tkn.lastChar != '=' {
				return tkn.scanBindVar()
			}
			fallthrough
		case '=', ',', ';', '(', ')', '+', '*', '&', '|', '^', '~', '[', ']', '?':
			return int(ch), []byte{byte(ch)}
		case '.':
			if isDigit(tkn.lastChar) {
				return tkn.scanNumber(true)
			}
			return int(ch), []byte{byte(ch)}
		case '/':
			switch tkn.lastChar {
			case '/':
				tkn.next()
				return tkn.scanCommentType1("//")
			case '*':
				tkn.next()
				return tkn.scanCommentType2()
			default:
				return int(ch), []byte{byte(ch)}
			}
		case '-':
			if tkn.lastChar == '-' {
				tkn.next()
				return tkn.scanCommentType1("--")
			}
			return int(ch), []byte{byte(ch)}
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
				return int(ch), []byte{byte(ch)}
			}
		case '>':
			if tkn.lastChar == '=' {
				tkn.next()
				return GE, []byte(">=")
			}
			return int(ch), []byte{byte(ch)}
		case '!':
			if tkn.lastChar == '=' {
				tkn.next()
				return NE, []byte("!=")
			}
			return LexError, []byte("!")
		case '\'':
			return tkn.scanString(ch, String)
		case '"':
			return tkn.scanString(ch, ID)
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
			return int(ch), []byte{byte(ch)}
		case '$':
			return tkn.scanPreparedStatement('$')
		case '{':
			return tkn.scanEscapeSequence('{')
		default:
			return LexError, []byte{byte(ch)}
		}
	}
}

func (tkn *Tokenizer) skipBlank() {
	ch := tkn.lastChar
	for ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
		tkn.next()
		ch = tkn.lastChar
	}
}

func (tkn *Tokenizer) scanIdentifier() (int, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteByte(byte(tkn.lastChar))
	tkn.next()

	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' || tkn.lastChar == '*' {
		buffer.WriteByte(byte(tkn.lastChar))
		tkn.next()
	}
	upper := bytes.ToUpper(buffer.Bytes())
	if keywordID, found := keywords[string(upper)]; found {
		return keywordID, upper
	}
	return ID, buffer.Bytes()
}

func (tkn *Tokenizer) scanLiteralIdentifier(quote rune) (int, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteByte(byte(tkn.lastChar))
	if !isLetter(tkn.lastChar) {
		return LexError, buffer.Bytes()
	}
	for tkn.next(); skipNonLiteralIdentifier(tkn.lastChar); tkn.next() {
		buffer.WriteByte(byte(tkn.lastChar))
	}
	// literals identifier are enclosed between quotes
	if tkn.lastChar != uint16(quote) {
		return LexError, buffer.Bytes()
	}
	tkn.next()
	return ID, buffer.Bytes()
}

func (tkn *Tokenizer) scanVariableIdentifier(prefix rune) (int, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(prefix)
	buffer.WriteByte(byte(tkn.lastChar))

	// expects that the variable is enclosed between '(' and ')' parenthesis
	if tkn.lastChar != '(' {
		return LexError, buffer.Bytes()
	}
	for tkn.next(); tkn.lastChar != ')' && tkn.lastChar != EOFChar; tkn.next() {
		buffer.WriteByte(byte(tkn.lastChar))
	}

	buffer.WriteByte(byte(tkn.lastChar))
	tkn.next()
	buffer.WriteByte(byte(tkn.lastChar))
	if !isLetter(tkn.lastChar) {
		return LexError, buffer.Bytes()
	}
	tkn.next()
	return Variable, buffer.Bytes()
}

func (tkn *Tokenizer) scanFormatParameter(prefix rune) (int, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteRune(prefix)
	buffer.WriteByte(byte(tkn.lastChar))

	tkn.next()
	return Variable, buffer.Bytes()
}

func (tkn *Tokenizer) scanPreparedStatement(prefix rune) (int, []byte) {
	buffer := &bytes.Buffer{}

	// a prepared statement expect a digit identifier like $1
	if !isDigit(tkn.lastChar) {
		return LexError, buffer.Bytes()
	}

	// read numbers and return an error if any
	token, buff := tkn.scanNumber(false)
	if token == LexError {
		return LexError, buffer.Bytes()
	}

	buffer.WriteRune(prefix)
	buffer.Write(buff)
	return PreparedStatement, buffer.Bytes()
}

func (tkn *Tokenizer) scanEscapeSequence(braces rune) (int, []byte) {
	buffer := &bytes.Buffer{}
	buffer.WriteByte(byte(braces))

	for tkn.lastChar != '}' && tkn.lastChar != EOFChar {
		buffer.WriteByte(byte(tkn.lastChar))
		tkn.next()
	}

	// we've reached the end of the string without finding
	// the closing curly braces
	if tkn.lastChar == EOFChar {
		return LexError, buffer.Bytes()
	}

	buffer.WriteByte(byte(tkn.lastChar))
	tkn.next()
	return EscapeSequence, buffer.Bytes()
}

func (tkn *Tokenizer) scanBindVar() (int, []byte) {
	buffer := bytes.NewBufferString(":")
	token := ValueArg
	if tkn.lastChar == ':' {
		token = ListArg
		buffer.WriteByte(byte(tkn.lastChar))
		tkn.next()
	}
	if !isLetter(tkn.lastChar) {
		return LexError, buffer.Bytes()
	}
	for isLetter(tkn.lastChar) || isDigit(tkn.lastChar) || tkn.lastChar == '.' {
		buffer.WriteByte(byte(tkn.lastChar))
		tkn.next()
	}
	return token, buffer.Bytes()
}

func (tkn *Tokenizer) scanMantissa(base int, buffer *bytes.Buffer) {
	for digitVal(tkn.lastChar) < base {
		tkn.consumeNext(buffer)
	}
}

func (tkn *Tokenizer) scanNumber(seenDecimalPoint bool) (int, []byte) {
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
	return Number, buffer.Bytes()
}

func (tkn *Tokenizer) scanString(delim uint16, typ int) (int, []byte) {
	buffer := &bytes.Buffer{}
	for {
		ch := tkn.lastChar
		tkn.next()
		if ch == delim {
			if tkn.lastChar == delim {
				tkn.next()
			} else {
				break
			}
		} else if ch == '\\' {
			if tkn.lastChar == EOFChar {
				return LexError, buffer.Bytes()
			}

			ch = tkn.lastChar
			tkn.next()
		}
		if ch == EOFChar {
			return LexError, buffer.Bytes()
		}
		buffer.WriteByte(byte(ch))
	}
	buf := buffer.Bytes()
	if typ == ID && len(buf) == 0 || bytes.IndexFunc(buf, func(r rune) bool { return !unicode.IsSpace(r) }) == -1 {
		// This string is an empty or white-space only identifier.
		// We should keep the start and end delimiters in order to
		// avoid creating invalid queries.
		// See: https://github.com/DataDog/datadog-trace-agent/issues/316
		return typ, []byte{byte(delim), byte(delim)}
	}
	return typ, buf
}

func (tkn *Tokenizer) scanCommentType1(prefix string) (int, []byte) {
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

func (tkn *Tokenizer) scanCommentType2() (int, []byte) {
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
			return LexError, buffer.Bytes()
		}
		tkn.consumeNext(buffer)
	}
	return Comment, buffer.Bytes()
}

func (tkn *Tokenizer) consumeNext(buffer *bytes.Buffer) {
	if tkn.lastChar == EOFChar {
		// This should never happen.
		panic("unexpected EOF")
	}
	buffer.WriteByte(byte(tkn.lastChar))
	tkn.next()
}

func (tkn *Tokenizer) next() {
	if ch, err := tkn.InStream.ReadByte(); err != nil {
		// Only EOF is possible.
		tkn.lastChar = EOFChar
	} else {
		tkn.lastChar = uint16(ch)
	}
	tkn.Position++
}

func skipNonLiteralIdentifier(ch uint16) bool {
	return isLetter(ch) || isDigit(ch) || '.' == ch || '-' == ch
}

func isLeadingLetter(ch uint16) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch == '@'
}

func isLetter(ch uint16) bool {
	return isLeadingLetter(ch) || ch == '#'
}

func digitVal(ch uint16) int {
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

func isDigit(ch uint16) bool {
	return '0' <= ch && ch <= '9'
}
