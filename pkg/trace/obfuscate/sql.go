// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/twmb/murmur3"
)

const sqlQueryTag = "sql.query"
const nonParsableResource = "Non-parsable SQL query"

// tokenFilter is a generic interface that a sqlObfuscator expects. It defines
// the Filter() function used to filter or replace given tokens.
// A filter can be stateful and keep an internal state to apply the filter later;
// this can be useful to prevent backtracking in some cases.

// tokenFilter implementations process tokens in the order that they are found by the tokenizer
// and respond as to how they should be interpreted in the result. For example: one token filter
// may decide that a token should be hidden in the result, or that it should be transformed in
// some way.
type tokenFilter interface {
	// Filter takes the current token kind, the last token kind and the token itself,
	// returning the new token kind and the value that should be stored in the final query,
	// along with an error.
	Filter(token, lastToken TokenKind, buffer []byte) (TokenKind, []byte, error)

	// Reset resets the filter.
	Reset()
}

// discardFilter is a token filter which discards certain elements from a query, such as
// comments and AS aliases by returning a nil buffer.
type discardFilter struct{}

// Filter the given token so that a `nil` slice is returned if the token is in the token filtered list.
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

// Reset implements tokenFilter.
func (f *discardFilter) Reset() {}

// replaceFilter is a token filter which obfuscates strings and numbers in queries by replacing them
// with the "?" character.
type replaceFilter struct{}

// Filter the given token so that it will be replaced if in the token replacement list
func (f *replaceFilter) Filter(token, lastToken TokenKind, buffer []byte) (tokenType TokenKind, tokenBytes []byte, err error) {
	switch lastToken {
	case Savepoint:
		return FilteredGroupable, []byte("?"), nil
	case '=':
		switch token {
		case DoubleQuotedString:
			// double-quoted strings after assignments are eligible for obfuscation
			return FilteredGroupable, []byte("?"), nil
		}
	}
	switch token {
	case String, Number, Null, Variable, PreparedStatement, BooleanLiteral, EscapeSequence:
		return FilteredGroupable, []byte("?"), nil
	default:
		return token, buffer, nil
	}
}

// Reset implements tokenFilter.
func (f *replaceFilter) Reset() {}

// groupingFilter is a token filter which groups together items replaced by the replaceFilter. It is meant
// to run immediately after it.
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

// Reset resets the groupingFilter so that it may be used again.
func (f *groupingFilter) Reset() {
	f.groupFilter = 0
	f.groupMulti = 0
}

// ObfuscateSQLString quantizes and obfuscates the given input SQL query string. Quantization removes
// some elements such as comments and aliases and obfuscation attempts to hide sensitive information
// in strings and numbers by redacting them.
func (o *Obfuscator) ObfuscateSQLString(in string) (*ObfuscatedQuery, error) {
	lesc := o.SQLLiteralEscapes()
	tok := NewSQLTokenizer(in, lesc)
	out, err := attemptObfuscation(tok)
	if err != nil && tok.SeenEscape() {
		// If the tokenizer failed, but saw an escape character in the process,
		// try again treating escapes differently
		tok = NewSQLTokenizer(in, !lesc)
		if out, err2 := attemptObfuscation(tok); err2 == nil {
			// If the second attempt succeeded, change the default behavior so that
			// on the next run we get it right in the first run.
			o.SetSQLLiteralEscapes(!lesc)
			return out, nil
		}
	}
	return out, err
}

// tableFinderFilter is a filter which attempts to identify the table name as it goes through each
// token in a query.
type tableFinderFilter struct {
	// seen keeps track of unique table names encountered by the filter.
	seen map[string]struct{}
	// csv specifies a comma-separated list of tables
	csv strings.Builder
}

// Filter implements tokenFilter.
func (f *tableFinderFilter) Filter(token, lastToken TokenKind, buffer []byte) (TokenKind, []byte, error) {
	switch lastToken {
	case From:
		// SELECT ... FROM [tableName]
		// DELETE FROM [tableName]
		if r, _ := utf8.DecodeRune(buffer); !unicode.IsLetter(r) {
			// first character in buffer is not a letter; we might have a nested
			// query like SELECT * FROM (SELECT ...)
			break
		}
		fallthrough
	case Update, Into, Join:
		// UPDATE [tableName]
		// INSERT INTO [tableName]
		// ... JOIN [tableName]
		f.storeName(string(buffer))
	}
	return token, buffer, nil
}

// storeName marks the given table name as seen in the internal storage.
func (f *tableFinderFilter) storeName(name string) {
	if _, ok := f.seen[name]; ok {
		return
	}
	if f.seen == nil {
		f.seen = make(map[string]struct{}, 1)
	}
	f.seen[name] = struct{}{}
	if f.csv.Len() > 0 {
		f.csv.WriteByte(',')
	}
	f.csv.WriteString(name)
}

// CSV returns a comma-separated list of the tables seen by the filter.
func (f *tableFinderFilter) CSV() string { return f.csv.String() }

// Reset implements tokenFilter.
func (f *tableFinderFilter) Reset() {
	for k := range f.seen {
		delete(f.seen, k)
	}
	f.csv.Reset()
}

// ObfuscatedQuery specifies information about an obfuscated SQL query.
type ObfuscatedQuery struct {
	Query     string // the obfuscated SQL query
	TablesCSV string // comma-separated list of tables that the query addresses
}

// attemptObfuscation attempts to obfuscate the SQL query loaded into the tokenizer, using the
// given set of filters.
func attemptObfuscation(tokenizer *SQLTokenizer) (*ObfuscatedQuery, error) {
	filters := []tokenFilter{
		&discardFilter{},
		&replaceFilter{},
		&groupingFilter{},
	}
	tableFinder := &tableFinderFilter{}
	if config.HasFeature("table_names") {
		filters = append(filters, tableFinder)
	}
	var (
		out       bytes.Buffer
		err       error
		lastToken TokenKind
	)
	// call Scan() function until tokens are available or if a LEX_ERROR is raised. After
	// retrieving a token, send it to the tokenFilter chains so that the token is discarded
	// or replaced.
	for {
		token, buff := tokenizer.Scan()
		if token == EOFChar {
			break
		}
		if token == LexError {
			return nil, fmt.Errorf("%v", tokenizer.Err())
		}
		for _, f := range filters {
			if token, buff, err = f.Filter(token, lastToken, buff); err != nil {
				return nil, err
			}
		}
		if buff != nil {
			if out.Len() != 0 {
				switch token {
				case ',':
				case '=':
					if lastToken == ':' {
						// do not add a space before an equals if a colon was
						// present before it.
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
		return nil, errors.New("result is empty")
	}
	return &ObfuscatedQuery{
		Query:     out.String(),
		TablesCSV: tableFinder.CSV(),
	}, nil
}

func (o *Obfuscator) obfuscateSQL(span *pb.Span) {
	tags := []string{"type:sql"}
	defer func() {
		metrics.Count("datadog.trace_agent.obfuscations", 1, tags, 1)
	}()
	if span.Resource == "" {
		tags = append(tags, "outcome:empty-resource")
		return
	}
	oq, err := o.ObfuscateSQLString(span.Resource)
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
	span.Resource = oq.Query

	if len(oq.TablesCSV) > 0 {
		traceutil.SetMeta(span, "sql.tables", oq.TablesCSV)
	}
	if span.Meta != nil && span.Meta[sqlQueryTag] != "" {
		// "sql.query" tag already set by user, do not change it.
		return
	}
	traceutil.SetMeta(span, sqlQueryTag, oq.Query)
}

// HashObfuscatedSQL returns the hash of an already obfuscated query as 32 char hex string
// the query must already have been obfuscated using ObfuscateSQLString
func HashObfuscatedSQL(obfuscatedSQL string) string {
	return strconv.FormatUint(murmur3.Sum64([]byte(obfuscatedSQL)), 16)
}
