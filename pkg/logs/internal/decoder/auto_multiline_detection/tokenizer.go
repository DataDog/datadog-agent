// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"bytes"
	"strings"
	"unicode"
)

// Token is the type that represents a single token.
type Token byte

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 below.
const maxRun = 10

const (
	space Token = iota

	// Special Characters
	colon        // :
	semicolon    // ;
	dash         // -
	underscore   // _
	fslash       // /
	bslash       // \
	period       // .
	comma        // ,
	singlequote  // '
	doublequote  // "
	backtick     // `
	tilda        // ~
	star         // *
	plus         // +
	equal        // =
	parenopen    // (
	parenclose   // )
	braceopen    // {
	braceclose   // }
	bracketopen  // [
	bracketclose // ]
	ampersand    // &
	exclamation  // !
	at           // @
	pound        // #
	dollar       // $
	percent      // %
	uparrow      // ^

	// Digit runs
	d1
	d2
	d3
	d4
	d5
	d6
	d7
	d8
	d9
	d10

	// Char runs
	c1
	c2
	c3
	c4
	c5
	c6
	c7
	c8
	c9
	c10

	// Special tokens
	month
	day
	apm  // am or pm
	zone // Represents a timezone
	t    // t (often `T`) denotes a time separator in many timestamp formats

	end // Not a valid token. Used to mark the end of the token list or as a terminator.
)

// Tokenizer is a heuristic to compute tokens from a log message.
// The tokenizer is used to convert a log message (string of bytes) into a list of tokens that
// represents the underlying structure of the log. The string of tokens is a compact slice of bytes
// that can be used to compare log messages structure. A tokenizer instance is not thread safe
// as bufferes are reused to avoid allocations.
type Tokenizer struct {
	maxEvalBytes int
	strBuf       *bytes.Buffer
}

// NewTokenizer returns a new Tokenizer detection heuristic.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	return &Tokenizer{
		maxEvalBytes: maxEvalBytes,
		strBuf:       bytes.NewBuffer(make([]byte, 0, maxRun)),
	}
}

// Process enriches the message context with tokens.
// This implements the Herustic interface - this heuristic does not stop processing.
func (t *Tokenizer) Process(context *messageContext) bool {
	maxBytes := len(context.rawMessage)
	if maxBytes > t.maxEvalBytes {
		maxBytes = t.maxEvalBytes
	}
	context.tokens = t.tokenize(context.rawMessage[:maxBytes])
	return true
}

// tokenize converts a byte slice to a list of tokens.
func (t *Tokenizer) tokenize(input []byte) []Token {
	// len(tokens) will always be <= len(input)
	tokens := make([]Token, 0, len(input))
	if len(input) == 0 {
		return tokens
	}

	run := 0
	lastToken := getToken(input[0])
	t.strBuf.Reset()
	t.strBuf.WriteRune(unicode.ToUpper(rune(input[0])))

	insertToken := func() {
		defer func() {
			run = 0
			t.strBuf.Reset()
		}()

		// Only test for special tokens if the last token was a charcater (Special tokens are currently only A-Z).
		if lastToken == c1 {
			if t.strBuf.Len() == 1 {
				if specialToken := getSpecialShortToken(t.strBuf.Bytes()[0]); specialToken != end {
					tokens = append(tokens, specialToken)
					return
				}
			} else if t.strBuf.Len() > 1 { // Only test special long tokens if buffer is > 1 token
				if specialToken := getSpecialLongToken(t.strBuf.String()); specialToken != end {
					tokens = append(tokens, specialToken)
					return
				}
			}
		}

		// Check for char or digit runs
		if lastToken == c1 || lastToken == d1 {
			// Limit max run size
			if run >= maxRun {
				run = maxRun - 1
			}
			tokens = append(tokens, lastToken+Token(run))
		} else {
			tokens = append(tokens, lastToken)
		}
	}

	for _, char := range input[1:] {
		currentToken := getToken(char)
		if currentToken != lastToken {
			insertToken()
		} else {
			run++
		}
		if currentToken == c1 {
			// Store upper case A-Z characters for matching special tokens
			t.strBuf.WriteRune(unicode.ToUpper(rune(char)))
		} else {
			t.strBuf.WriteByte(char)
		}
		lastToken = currentToken
	}

	// Flush any remaining buffered tokens
	insertToken()

	return tokens
}

// getToken returns a single token from a single byte.
func getToken(char byte) Token {
	if unicode.IsDigit(rune(char)) {
		return d1
	} else if unicode.IsSpace(rune(char)) {
		return space
	}

	switch char {
	case ':':
		return colon
	case ';':
		return semicolon
	case '-':
		return dash
	case '_':
		return underscore
	case '/':
		return fslash
	case '\\':
		return bslash
	case '.':
		return period
	case ',':
		return comma
	case '\'':
		return singlequote
	case '"':
		return doublequote
	case '`':
		return backtick
	case '~':
		return tilda
	case '*':
		return star
	case '+':
		return plus
	case '=':
		return equal
	case '(':
		return parenopen
	case ')':
		return parenclose
	case '{':
		return braceopen
	case '}':
		return braceclose
	case '[':
		return bracketopen
	case ']':
		return bracketclose
	case '&':
		return ampersand
	case '!':
		return exclamation
	case '@':
		return at
	case '#':
		return pound
	case '$':
		return dollar
	case '%':
		return percent
	case '^':
		return uparrow
	}

	return c1
}

func getSpecialShortToken(char byte) Token {
	switch char {
	case 'T':
		return t
	case 'Z':
		return zone
	}
	return end
}

// getSpecialLongToken returns a special token that is > 1 character.
// NOTE: This set of tokens is non-exhaustive and can be expanded.
func getSpecialLongToken(input string) Token {
	switch input {
	case "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL",
		"AUG", "SEP", "OCT", "NOV", "DEC":
		return month
	case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
		return day
	case "AM", "PM":
		return apm
	case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
		"MST", "MDT", "PST", "PDT", "JST", "KST",
		"IST", "MSK", "CEST", "CET", "BST", "NZST",
		"NZDT", "ACST", "ACDT", "AEST", "AEDT",
		"AWST", "AWDT", "AKST", "AKDT", "HST",
		"HDT", "CHST", "CHDT", "NST", "NDT":
		return zone
	}

	return end
}

// tokenToString converts a single token to a debug string.
func tokenToString(token Token) string {
	if token >= d1 && token <= d10 {
		return strings.Repeat("D", int(token-d1)+1)
	} else if token >= c1 && token <= c10 {
		return strings.Repeat("C", int(token-c1)+1)
	}

	switch token {
	case space:
		return " "
	case colon:
		return ":"
	case semicolon:
		return ";"
	case dash:
		return "-"
	case underscore:
		return "_"
	case fslash:
		return "/"
	case bslash:
		return "\\"
	case period:
		return "."
	case comma:
		return ","
	case singlequote:
		return "'"
	case doublequote:
		return "\""
	case backtick:
		return "`"
	case tilda:
		return "~"
	case star:
		return "*"
	case plus:
		return "+"
	case equal:
		return "="
	case parenopen:
		return "("
	case parenclose:
		return ")"
	case braceopen:
		return "{"
	case braceclose:
		return "}"
	case bracketopen:
		return "["
	case bracketclose:
		return "]"
	case ampersand:
		return "&"
	case exclamation:
		return "!"
	case at:
		return "@"
	case pound:
		return "#"
	case dollar:
		return "$"
	case percent:
		return "%"
	case uparrow:
		return "^"
	case month:
		return "MTH"
	case day:
		return "DAY"
	case apm:
		return "PM"
	case t:
		return "T"
	case zone:
		return "ZONE"
	}

	return ""
}

// tokensToString converts a list of tokens to a debug string.
func tokensToString(tokens []Token) string {
	str := ""
	for _, t := range tokens {
		str += tokenToString(t)
	}
	return str
}

// isMatch compares two sets of tokens and returns true if they match.
// if the token strings are different lengths, the shortest string is used for comparison.
func isMatch(setA []Token, setB []Token, thresh float64) bool {
	count := len(setA)
	if len(setB) < count {
		count = len(setB)
	}

	match := 0
	for i := 0; i < count; i++ {
		if setA[i] == setB[i] {
			match++
		}
	}

	return float64(match)/float64(count) >= thresh
}
