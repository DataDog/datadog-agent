// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// Names of the helper functions emitted by the translator for the SECL
// operators that have no CEL equivalent. Everything else maps onto standard CEL
// or onto the math and network extension libraries.
const (
	// GlobFunc reports whether a string matches a SECL glob pattern, in which
	// `*` matches any sequence of characters and the pattern must match the
	// whole string. CEL only offers RE2 matching, which SECL exposes separately
	// through `r"…"` literals.
	GlobFunc = "secl.glob"

	// CIDRMatchFunc implements SECL's `in`, `==` and `!=` between IP and CIDR
	// operands, which match when either range contains the base address of the
	// other rather than when one contains the other. It is equivalent to
	//
	//	a.masked().containsIP(b.masked().ip()) || b.masked().containsIP(a.masked().ip())
	//
	// and is overloaded on both single values and lists of values.
	CIDRMatchFunc = "secl.cidrMatch"

	// CIDRMatchAllFunc implements SECL's `allin` between IP and CIDR operands:
	// every pair drawn from the two sides must satisfy CIDRMatchFunc.
	CIDRMatchAllFunc = "secl.cidrMatchAll"

	// ElapsedFunc converts a nanosecond timestamp into the duration that has
	// passed since then, i.e. `now - value`. SECL applies this implicitly
	// whenever a field is compared against a duration literal.
	ElapsedFunc = "secl.elapsed"

	// NanosFunc reinterprets a nanosecond count as a duration. SECL applies this
	// instead of ElapsedFunc when the left hand side of a duration comparison is
	// itself an arithmetic expression, e.g. `a - b < 10m`.
	NanosFunc = "secl.nanos"

	// StrFunc renders a value as the string SECL would substitute for it inside
	// an interpolated string: integers are formatted in base 10 and lists are
	// joined with commas.
	StrFunc = "secl.str"

	// RootDomainFunc extracts the effective root domain of a host name. It backs
	// SECL's `%{field.root_domain}` suffix.
	RootDomainFunc = "secl.rootDomain"
)

// VariablesRoot is the CEL identifier that SECL variables are rooted at:
// `${foo.bar}` translates to `vars.foo.bar`. Fields keep their own names, so
// the prefix is what keeps a variable from colliding with a field of the same
// name.
const VariablesRoot = "vars"

// NewEnv returns a CEL environment that declares everything a translated SECL
// expression can refer to except the event fields, the macros and the
// variables, which depend on the model and are supplied by the caller through
// opts.
//
// The declared helper functions have no runtime binding yet: this environment
// type-checks translated expressions, it does not evaluate them.
func NewEnv(opts ...cel.EnvOption) (*cel.Env, error) {
	base := []cel.EnvOption{
		// math.bitAnd/bitOr/bitXor/bitNot back SECL's &, |, ^ operators.
		ext.Math(),
		// ip()/cidr() and their member functions back SECL's IP and CIDR
		// literals.
		ext.Network(),
		// `allin` and iterator variables translate to comprehensions, which can
		// only be printed back as all() and exists() calls when the macro call
		// metadata is kept.
		cel.EnableMacroCallTracking(),
		cel.Variable(VariablesRoot, cel.DynType),

		cel.Function(GlobFunc,
			cel.Overload("secl_glob_string_string", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType)),
		cel.Function(CIDRMatchFunc,
			cel.Overload("secl_cidr_match", []*cel.Type{cel.DynType, cel.DynType}, cel.BoolType)),
		cel.Function(CIDRMatchAllFunc,
			cel.Overload("secl_cidr_match_all", []*cel.Type{cel.DynType, cel.DynType}, cel.BoolType)),
		cel.Function(ElapsedFunc,
			cel.Overload("secl_elapsed_int", []*cel.Type{cel.IntType}, cel.DurationType)),
		cel.Function(NanosFunc,
			cel.Overload("secl_nanos_int", []*cel.Type{cel.IntType}, cel.DurationType)),
		cel.Function(StrFunc,
			cel.Overload("secl_str_dyn", []*cel.Type{cel.DynType}, cel.StringType)),
		cel.Function(RootDomainFunc,
			cel.Overload("secl_root_domain_string", []*cel.Type{cel.StringType}, cel.StringType)),
	}

	return cel.NewEnv(append(base, opts...)...)
}
