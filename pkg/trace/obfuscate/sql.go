// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package obfuscate

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
)

const sqlQueryTag = "sql.query"
const nonParsableResource = "Non-parsable SQL query"

// tokenFilter is a generic interface that a sqlObfuscator expects. It defines
// the Filter() function used to filter or replace given tokens.
// A filter can be stateful and keep an internal state to apply the filter later;
// this can be useful to prevent backtracking in some cases.
type tokenFilter interface {
	Filter(token, lastToken TokenKind, buffer []byte) (TokenKind, []byte, error)
	Reset()
}

// discardFilter implements the tokenFilter interface so that the given
// token is discarded or accepted.
type discardFilter struct{}

// Filter the given token so that a `nil` slice is returned if the token
// is in the token filtered list.
func (f *discardFilter) Filter(token, lastToken TokenKind, buffer []byte) (TokenKind, []byte, error) {
	// filters based on previous token
	switch lastToken {
	case FilteredBracketedIdentifier:
		if token != ']' {
			// we haven't found the closing bracket yet, keep going
			if token != ID {
				// the token between the brackets *must* be an identifier,
				// otherwise the query is invalid.
				return LexError, nil, fmt.Errorf("expected identifier in bracketed filter, got %d", token)
			}
			return FilteredBracketedIdentifier, nil, nil
		}
		fallthrough
	case As:
		if token == '[' {
			// the identifier followed by AS is an MSSQL bracketed identifier
			// and will continue to be discarded until we find the corresponding
			// closing bracket counter-part. See GitHub issue DataDog/datadog-trace-agent#475.
			return FilteredBracketedIdentifier, nil, nil
		}
		return Filtered, nil, nil
	}

	// filters based on the current token; if the next token should be ignored,
	// return the same token value (not FilteredGroupable) and nil
	switch token {
	case As:
		return As, nil, nil
	case Comment, ';':
		return FilteredGroupable, nil, nil
	default:
		return token, buffer, nil
	}
}

// Reset in a discardFilter is a noop action
func (f *discardFilter) Reset() {}

// replaceFilter implements the tokenFilter interface so that the given
// token is replaced with '?' or left unchanged.
type replaceFilter struct{}

// Filter the given token so that it will be replaced if in the token replacement list
func (f *replaceFilter) Filter(token, lastToken TokenKind, buffer []byte) (tokenType TokenKind, tokenBytes []byte, err error) {
	switch lastToken {
	case Savepoint:
		return FilteredGroupable, []byte("?"), nil
	case '=':
		switch token {
		case DoubleQuotedString, DoubleQuotedEscapedString:
			// double-quoted strings after assignments are eligible for obfuscation
			return FilteredGroupable, []byte("?"), nil
		}
	}
	switch token {
	case String, EscapedString, Number, Null, Variable, PreparedStatement, BooleanLiteral, EscapeSequence:
		return FilteredGroupable, []byte("?"), nil
	default:
		return token, buffer, nil
	}
}

// Reset in a replaceFilter is a noop action
func (f *replaceFilter) Reset() {}

// groupingFilter implements the tokenFilter interface so that when
// a common pattern is identified, it's discarded to prevent duplicates
type groupingFilter struct {
	groupFilter int
	groupMulti  int
}

// Filter the given token so that it will be discarded if a grouping pattern
// has been recognized. A grouping is composed by items like:
//   * '( ?, ?, ? )'
//   * '( ?, ? ), ( ?, ? )'
func (f *groupingFilter) Filter(token, lastToken TokenKind, buffer []byte) (tokenType TokenKind, tokenBytes []byte, err error) {
	// increasing the number of groups means that we're filtering an entire group
	// because it can be represented with a single '( ? )'
	if (lastToken == '(' && token == FilteredGroupable) || (token == '(' && f.groupMulti > 0) {
		f.groupMulti++
	}

	switch {
	case token == FilteredGroupable:
		// the previous filter has dropped this token so we should start
		// counting the group filter so that we accept only one '?' for
		// the same group
		f.groupFilter++

		if f.groupFilter > 1 {
			return FilteredGroupable, nil, nil
		}
	case f.groupFilter > 0 && (token == ',' || token == '?'):
		// if we are in a group drop all commas
		return FilteredGroupable, nil, nil
	case f.groupMulti > 1:
		// drop all tokens since we're in a counting group
		// and they're duplicated
		return FilteredGroupable, nil, nil
	case token != ',' && token != '(' && token != ')' && token != FilteredGroupable:
		// when we're out of a group reset the filter state
		f.Reset()
	}

	return token, buffer, nil
}

// Reset in a groupingFilter restores variables used to count
// escaped token that should be filtered
func (f *groupingFilter) Reset() {
	f.groupFilter = 0
	f.groupMulti = 0
}

// Process the given SQL or No-SQL string so that the resulting one is properly altered. This
// function is generic and the behavior changes according to chosen tokenFilter implementations.
// The process calls all filters inside the []tokenFilter.
func obfuscateSQLString(in string) (string, error) {
	tokenizer := NewSQLTokenizer(in)
	filters := []tokenFilter{&discardFilter{}, &replaceFilter{}, &groupingFilter{}}
	var (
		out       bytes.Buffer
		err       error
		lastToken TokenKind

		hasEscaped         bool
		backtrackPos       int       // position of Tokenizer when first escaped quote encountered
		backtrackOutLen    int       // length of output buffer when first escaped quote encountered
		backtrackLastToken TokenKind // value of lastToken when first escaped quote encountered
	)

	// call Scan() function until tokens are available or if a LEX_ERROR is raised. After
	// retrieving a token, send it to the tokenFilter chains so that the token is discarded
	// or replaced.
	for {
		pos := tokenizer.pos
		token, buff := tokenizer.Scan()
		if token == EOFChar {
			break
		}

		if (token == EscapedString || token == DoubleQuotedEscapedString) && !hasEscaped {
			// tokenizer has encountered its first escaped string
			hasEscaped = true
			backtrackPos = pos
			backtrackOutLen = out.Len()
			backtrackLastToken = lastToken
		}

		if token == LexError {
			// if at any point we've tried escaping a backslash-quote, e.g. \'
			// try re-lexing the input without escapes
			if !tokenizer.ignoreEscape && hasEscaped {
				// back up the reader and the output buffer
				tokenizer.Seek(int64(backtrackPos), io.SeekStart)
				out.Truncate(backtrackOutLen)
				lastToken = backtrackLastToken

				// try once more without escaping backslash quotes
				tokenizer.ignoreEscape = true
				continue
			}

			return "", tokenizer.Err()
		}

		for _, f := range filters {
			if token, buff, err = f.Filter(token, lastToken, buff); err != nil {
				return "", err
			}
		}
		if buff != nil {
			if out.Len() != 0 {
				switch token {
				case ',':
				case '=':
					if lastToken == ':' {
						break
					}
					fallthrough
				default:
					out.WriteRune(' ')
				}
			}
			out.Write(buff)
		}
		lastToken = token
	}
	if out.Len() == 0 {
		return "", errors.New("result is empty")
	}
	return out.String(), nil
}

// QuantizeSQL generates resource and sql.query meta for SQL spans
func (o *Obfuscator) obfuscateSQL(span *pb.Span) {
	tags := []string{"type:sql"}
	defer func() {
		metrics.Count("datadog.trace_agent.obfuscations", 1, tags, 1)
	}()
	if span.Resource == "" {
		tags = append(tags, "outcome:empty-resource")
		return
	}
	result, err := obfuscateSQLString(span.Resource)
	if err != nil {
		// we have an error, discard the SQL to avoid polluting user resources.
		log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
		if span.Meta == nil {
			span.Meta = make(map[string]string, 1)
		}
		if _, ok := span.Meta[sqlQueryTag]; !ok {
			span.Meta[sqlQueryTag] = span.Resource
		}
		span.Resource = nonParsableResource
		tags = append(tags, "outcome:error")
		return
	}

	tags = append(tags, "outcome:success")
	span.Resource = result

	if span.Meta != nil && span.Meta[sqlQueryTag] != "" {
		// "sql.query" tag already set by user, do not change it.
		return
	}
	if span.Meta == nil {
		span.Meta = make(map[string]string)
	}
	span.Meta[sqlQueryTag] = result
}
