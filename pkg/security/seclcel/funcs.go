// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// errUnsupportedValue is the base for the errors a helper returns when handed a
// value of a shape it cannot work with, which the checker should have prevented.
var errUnsupportedValue = errors.New("seclcel")

// The helper implementations delegate to pkg/security/secl/compiler/eval rather
// than reimplementing what it already does, so that glob matching and the CIDR
// overlap rule have one definition shared with the SECL evaluator.

// helperBindings implements the functions NewEnv declares.
func helperBindings() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function(GlobFunc,
			cel.Overload("secl_glob_string_string", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType,
				cel.BinaryBinding(celGlob))),

		cel.Function(CIDRMatchFunc,
			cel.Overload("secl_cidr_match", []*cel.Type{cel.DynType, cel.DynType}, cel.BoolType,
				cel.BinaryBinding(celCIDRMatchAny))),

		cel.Function(CIDRMatchAllFunc,
			cel.Overload("secl_cidr_match_all", []*cel.Type{cel.DynType, cel.DynType}, cel.BoolType,
				cel.BinaryBinding(celCIDRMatchAll))),

		cel.Function(NanosFunc,
			cel.Overload("secl_nanos_int", []*cel.Type{cel.IntType}, cel.DurationType,
				cel.UnaryBinding(celNanos))),

		cel.Function(StrFunc,
			cel.Overload("secl_str_dyn", []*cel.Type{cel.DynType}, cel.StringType,
				cel.UnaryBinding(celStr))),

		cel.Function(RootDomainFunc,
			cel.Overload("secl_root_domain_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(celRootDomain))),
	}
}

// celGlob matches a string against a SECL glob pattern.
//
// The case insensitive form SECL uses for some fields and platforms is not
// reproduced yet: this always matches case sensitively.
func celGlob(subject, pattern ref.Val) ref.Val {
	s, ok := subject.(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(subject)
	}
	p, ok := pattern.(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(pattern)
	}

	return types.Bool(eval.PatternMatches(string(p), string(s), false))
}

// celCIDRMatchAny implements SECL's `in`, `==` and `!=` between IP and CIDR
// operands: true when any pair drawn from the two sides matches.
func celCIDRMatchAny(lhs, rhs ref.Val) ref.Val {
	return cidrCompare(lhs, rhs, false)
}

// celCIDRMatchAll implements SECL's `allin`: every pair drawn from the two sides
// must match, which is what eval.CIDRValues.MatchAll does.
func celCIDRMatchAll(lhs, rhs ref.Val) ref.Val {
	return cidrCompare(lhs, rhs, true)
}

func cidrCompare(lhs, rhs ref.Val, all bool) ref.Val {
	left, err := ipNets(lhs)
	if err != nil {
		return types.WrapErr(err)
	}
	right, err := ipNets(rhs)
	if err != nil {
		return types.WrapErr(err)
	}

	// An empty side has no pair to match, so it matches nothing either way.
	if len(left) == 0 || len(right) == 0 {
		return types.Bool(false)
	}

	for _, l := range left {
		for _, r := range right {
			// eval.IPNetsMatch is SECL's rule: either range containing the base
			// address of the other, rather than one containing the other.
			if eval.IPNetsMatch(l, r) != all {
				return types.Bool(!all)
			}
		}
	}
	return types.Bool(all)
}

// ipNets converts an IP, a CIDR or a list of either into the form
// eval.IPNetsMatch compares.
func ipNets(val ref.Val) ([]*net.IPNet, error) {
	switch v := val.(type) {
	case ext.CIDR:
		return []*net.IPNet{prefixToIPNet(v.Prefix)}, nil
	case ext.IP:
		return []*net.IPNet{eval.IPNetFromIP(net.IP(v.Addr.AsSlice()))}, nil
	case traits.Lister:
		var nets []*net.IPNet
		for it := v.Iterator(); it.HasNext() == types.True; {
			elem, err := ipNets(it.Next())
			if err != nil {
				return nil, err
			}
			nets = append(nets, elem...)
		}
		return nets, nil
	}
	return nil, fmt.Errorf("%w: %s is not an IP or a CIDR", errUnsupportedValue, val.Type())
}

// prefixToIPNet converts to the masked form net.ParseCIDR produces, which is
// what SECL compares: eval.IPNetsMatch reads the base address of each range.
func prefixToIPNet(prefix netip.Prefix) *net.IPNet {
	masked := prefix.Masked()
	addr := masked.Addr()

	return &net.IPNet{
		IP:   net.IP(addr.AsSlice()),
		Mask: net.CIDRMask(masked.Bits(), addr.BitLen()),
	}
}

// celNanos reinterprets a nanosecond count as a duration, which is how a SECL
// duration comparison reaches CEL.
func celNanos(val ref.Val) ref.Val {
	nanos, ok := val.(types.Int)
	if !ok {
		return types.MaybeNoSuchOverloadErr(val)
	}
	return types.Duration{Duration: time.Duration(nanos)}
}

// celStr renders a value the way SECL substitutes it into an interpolated
// string: integers in base 10 and lists joined with commas.
func celStr(val ref.Val) ref.Val {
	switch v := val.(type) {
	case types.String:
		return v
	case types.Int:
		return types.String(strconv.FormatInt(int64(v), 10))
	case types.Bool:
		return types.String(strconv.FormatBool(bool(v)))
	case traits.Lister:
		var parts []string
		for it := v.Iterator(); it.HasNext() == types.True; {
			part := celStr(it.Next())
			if types.IsError(part) {
				return part
			}
			parts = append(parts, string(part.(types.String)))
		}
		return types.String(strings.Join(parts, ","))
	}
	return types.WrapErr(fmt.Errorf("%w: %s cannot be rendered as a string", errUnsupportedValue, val.Type()))
}
