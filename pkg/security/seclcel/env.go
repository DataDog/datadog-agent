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

	// NanosFunc reinterprets a nanosecond count as a duration, which is how a
	// SECL duration comparison reaches CEL. SECL reads `field < 10m` as "less
	// than ten minutes ago", so the translation subtracts the field from NowVar
	// before applying it.
	NanosFunc = "secl.nanos"

	// StrFunc renders a value as the string SECL would substitute for it inside
	// an interpolated string: integers are formatted in base 10 and lists are
	// joined with commas.
	StrFunc = "secl.str"

	// RootDomainFunc extracts the effective root domain of a host name. It backs
	// SECL's `%{field.root_domain}` suffix.
	RootDomainFunc = "secl.rootDomain"
)

// globPatternArg is the argument of GlobFunc that holds the pattern, which is
// what lets the pattern be compiled at planning time when it is a literal.
const globPatternArg = 1

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
// The helper functions are bound, so an expression built from this environment
// can be evaluated against an event with Program and NewActivation.
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
		cel.Variable(NowVar, cel.IntType),
	}

	// The helpers are declared together with their implementations, so an
	// environment is evaluable as well as checkable.
	base = append(base, helperBindings()...)

	return cel.NewEnv(append(base, opts...)...)
}
