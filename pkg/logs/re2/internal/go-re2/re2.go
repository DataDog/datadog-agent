// Vendored from github.com/wasilibs/go-re2 v1.10.0
// See LICENSE in this directory for the original MIT license.

//go:build re2_cgo

package gre2

import (
	"github.com/DataDog/datadog-agent/pkg/logs/re2/internal/go-re2/internal"
)

type Regexp = internal.Regexp

// Compile parses a regular expression and returns, if successful,
// a Regexp object that can be used to match against text.
//
// When matching against text, the regexp returns a match that
// begins as early as possible in the input (leftmost), and among those
// it chooses the one that a backtracking search would have found first.
// This so-called leftmost-first matching is the same semantics
// that Perl, Python, and other implementations use, although this
// package implements it without the expense of backtracking.
func Compile(expr string) (*Regexp, error) {
	return internal.Compile(expr, internal.CompileOptions{})
}
