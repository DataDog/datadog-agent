// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/functions"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// A SECL pattern is a value here, not only an operator.
//
// `x == ~"/etc/*"` needs no value: the translation knows both sides and emits a glob
// call. A *list* of patterns does — `x in [ ~"/etc/*", "/etc/passwd" ]` — because what
// a member means is decided per member, and because a macro is a list a rule refers to
// by name, so the pattern-ness has to survive being handed around.
//
// So a glob and a regexp are values carrying their compiled matcher, a list of them is
// a prepared set, and membership is one call over that set. What that replaces is a
// disjunction with one term per member, each re-reading the subject.
//
// A pattern value carries *both* of the matchers SECL compiles a pattern literal into,
// because which one applies is a property of the field it is compared against and not
// of the literal — see celGlobFields. The same macro is used against a path field and
// against a plain string field, and SECL resolves the difference per comparison, after
// inlining; here one prepared set serves both and the membership call says which
// semantics to search it with.
//
// The types are opaque: nothing in CEL can be done with them except hand them to
// MatchAnyFunc, which is what keeps `process.uid in SHELL_NAMES` a type error rather than a
// rule that never fires.
var (
	// GlobType is a SECL glob pattern, compiled.
	GlobType = types.NewOpaqueType("secl.Glob")
	// RegexpType is a SECL regexp, compiled.
	RegexpType = types.NewOpaqueType("secl.Regexp")
	// PatternsType is what a list of strings, globs and regexps is prepared into.
	PatternsType = types.NewOpaqueType("secl.Patterns")
)

// globValue is a compiled SECL pattern literal, in both of the forms one can take.
//
// Neither is guaranteed to compile: `**` is refused as a pattern and accepted as a
// glob, and a malformed glob is refused as a glob and accepted as a pattern. So each
// form keeps the error that stopped it, to be reported if a rule turns out to need it —
// which is a rule that does not load, since a membership test resolves its semantics
// when it is planned.
type globValue struct {
	pattern string

	asPattern    eval.PatternStringMatcher
	patternError error
	asGlob       eval.GlobStringMatcher
	globError    error
}

func newGlobValue(pattern string) (ref.Val, error) {
	value := &globValue{pattern: pattern}

	value.patternError = value.asPattern.Compile(pattern, false)
	// The case insensitive and separator normalising forms SECL uses for some fields
	// and platforms are not reproduced yet — see the divergences in the package doc.
	value.globError = value.asGlob.Compile(pattern, false, false)

	if value.patternError != nil && value.globError != nil {
		return nil, fmt.Errorf("%w: %q is not a valid pattern: %w", errUnsupportedValue, pattern, value.patternError)
	}
	return value, nil
}

func (g *globValue) matches(s string, glob bool) bool {
	if glob {
		return g.asGlob.Matches(s)
	}
	return g.asPattern.Matches(s)
}

// supports reports the reason the value cannot be matched with the given semantics, or
// nil when it can.
func (g *globValue) supports(glob bool) error {
	if glob {
		if g.globError != nil {
			return fmt.Errorf("%w: %q is not a valid glob: %w", errUnsupportedValue, g.pattern, g.globError)
		}
		return nil
	}
	if g.patternError != nil {
		return fmt.Errorf("%w: %q is not a valid pattern: %w", errUnsupportedValue, g.pattern, g.patternError)
	}
	return nil
}

// Type implements ref.Val.
func (g *globValue) Type() ref.Type { return GlobType }

// Value implements ref.Val, returning the pattern as it was written.
func (g *globValue) Value() any { return g.pattern }

// Equal implements ref.Val as identity.
//
// A pattern is matched, never compared — how it matches depends on the field on the other
// side, which an equality does not carry — and nothing translates to an equality over one:
// a comparison becomes a glob call and a membership becomes MatchAnyFunc, both of which
// know the field. So what this answers only has to be reflexive, which is what cel-go
// requires of a value it has declared: Env.Extend refuses a constant that does not equal
// itself, reporting it as a conflicting definition.
func (g *globValue) Equal(other ref.Val) ref.Val {
	return types.Bool(g == other)
}

// ConvertToNative implements ref.Val.
func (g *globValue) ConvertToNative(typeDesc reflect.Type) (any, error) {
	if typeDesc.Kind() == reflect.String {
		return g.pattern, nil
	}
	return nil, fmt.Errorf("%w: a pattern cannot be converted to %s", errUnsupportedValue, typeDesc)
}

// ConvertToType implements ref.Val.
func (g *globValue) ConvertToType(t ref.Type) ref.Val {
	switch t {
	case types.StringType:
		return types.String(g.pattern)
	case types.TypeType:
		return GlobType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", GlobType, t)
}

// regexpValue is a compiled SECL regexp. SECL and CEL agree on the semantics —
// unanchored RE2 — so this exists for the same reason globValue does, to be a member of
// a list rather than an operator.
type regexpValue struct {
	pattern string
	re      *regexp.Regexp
}

func newRegexpValue(pattern string) (ref.Val, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is not a valid regexp: %w", errUnsupportedValue, pattern, err)
	}
	return &regexpValue{pattern: pattern, re: re}, nil
}

// matches implements matcher. A regexp is the one member kind whose meaning does not
// depend on the field: SECL compiles a `r"…"` literal the same way everywhere.
func (r *regexpValue) matches(s string, _ bool) bool { return r.re.MatchString(s) }

// supports implements matcher.
func (r *regexpValue) supports(bool) error { return nil }

// Type implements ref.Val.
func (r *regexpValue) Type() ref.Type { return RegexpType }

// Value implements ref.Val.
func (r *regexpValue) Value() any { return r.pattern }

// Equal implements ref.Val as identity — see globValue.Equal.
func (r *regexpValue) Equal(other ref.Val) ref.Val {
	return types.Bool(r == other)
}

// ConvertToNative implements ref.Val.
func (r *regexpValue) ConvertToNative(typeDesc reflect.Type) (any, error) {
	if typeDesc.Kind() == reflect.String {
		return r.pattern, nil
	}
	return nil, fmt.Errorf("%w: a regexp cannot be converted to %s", errUnsupportedValue, typeDesc)
}

// ConvertToType implements ref.Val.
func (r *regexpValue) ConvertToType(t ref.Type) ref.Val {
	switch t {
	case types.StringType:
		return types.String(r.pattern)
	case types.TypeType:
		return RegexpType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", RegexpType, t)
}

// matcher is what a prepared set holds for a member that is not a plain string.
type matcher interface {
	// matches reports whether the subject matches, under glob semantics or pattern
	// semantics — see the note at the top of this file.
	matches(s string, glob bool) bool
	// supports reports why the member cannot be matched under those semantics, if it
	// cannot. It is asked once, when a rule is planned.
	supports(glob bool) error
	Value() any
}

// patternSet is a list of members prepared for membership: the plain strings in a map,
// everything else as a compiled matcher.
//
// It is built when the rule is planned, so the map is hashed once and each pattern is
// compiled once, however many rules refer to the same macro.
type patternSet struct {
	strings  map[string]struct{}
	matchers []matcher
}

func newPatternSet(members ref.Val) (ref.Val, error) {
	list, ok := members.(traits.Lister)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not a list of patterns", errUnsupportedValue, members.Type())
	}

	set := &patternSet{strings: map[string]struct{}{}}
	for it := list.Iterator(); it.HasNext() == types.True; {
		switch member := it.Next().(type) {
		case types.String:
			set.strings[string(member)] = struct{}{}
		case matcher:
			set.matchers = append(set.matchers, member)
		default:
			return nil, fmt.Errorf("%w: %s cannot be a member of a pattern list",
				errUnsupportedValue, member.Type())
		}
	}
	return set, nil
}

// contains reports whether the subject equals one of the plain strings or matches one
// of the patterns, which is SECL's `in` over a list holding either.
func (p *patternSet) contains(s string, glob bool) bool {
	if _, ok := p.strings[s]; ok {
		return true
	}
	for _, m := range p.matchers {
		if m.matches(s, glob) {
			return true
		}
	}
	return false
}

// supports reports why the set cannot be searched under the given semantics, if it
// cannot: a member written with `**` can only be a glob, and a malformed glob can only
// be a pattern.
func (p *patternSet) supports(glob bool) error {
	for _, m := range p.matchers {
		if err := m.supports(glob); err != nil {
			return err
		}
	}
	return nil
}

// Type implements ref.Val.
func (p *patternSet) Type() ref.Type { return PatternsType }

// Value implements ref.Val.
func (p *patternSet) Value() any { return p }

// Equal implements ref.Val as identity. A set exists to be searched rather than compared,
// and identity is what a declared value has to answer — see globValue.Equal.
func (p *patternSet) Equal(other ref.Val) ref.Val {
	return types.Bool(p == other)
}

// ConvertToNative implements ref.Val.
func (p *patternSet) ConvertToNative(typeDesc reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: a pattern list cannot be converted to %s", errUnsupportedValue, typeDesc)
}

// ConvertToType implements ref.Val.
func (p *patternSet) ConvertToType(t ref.Type) ref.Val {
	switch t {
	case types.StringType:
		return types.String(p.String())
	case types.TypeType:
		return PatternsType
	}
	return types.NewErr("type conversion error from '%s' to '%s'", PatternsType, t)
}

// String renders the set the way the rule wrote it, for an error message and for the
// coverage tool.
func (p *patternSet) String() string {
	members := make([]string, 0, len(p.strings)+len(p.matchers))
	for s := range p.strings {
		members = append(members, fmt.Sprintf("%q", s))
	}
	for _, m := range p.matchers {
		members = append(members, fmt.Sprintf("%v", m.Value()))
	}
	sort.Strings(members)
	return "[" + strings.Join(members, ", ") + "]"
}

// patternBindings declares the pattern values and the membership test over them.
//
// GlobFunc carries a second, unary overload: with two arguments it matches, with one it
// builds the value. They are the same function to a rule author, and the translation
// picks by position — a comparison takes the first, a list member the second.
func patternBindings() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function(GlobFunc,
			cel.Overload("secl_glob_value", []*cel.Type{cel.StringType}, GlobType,
				cel.UnaryBinding(celGlobValue))),

		cel.Function(RegexpFunc,
			cel.Overload("secl_regexp_value", []*cel.Type{cel.StringType}, RegexpType,
				cel.UnaryBinding(celRegexpValue))),

		cel.Function(PatternsFunc,
			cel.Overload("secl_patterns", []*cel.Type{cel.ListType(cel.DynType)}, PatternsType,
				cel.UnaryBinding(celPatterns))),

		// The pattern form of a comparison against a path field, which is the same
		// function with the other matcher — see celGlobFields. There is no value
		// constructor for it: a pattern value carries both forms, and the membership
		// call below is what picks.
		cel.Function(PathGlobFunc,
			cel.Overload("secl_path_glob_string_string", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType,
				cel.BinaryBinding(celPathGlob))),

		// The overloads are what keeps membership type-checked: a set of patterns and
		// a list of strings can both be searched for a string, and nothing can be
		// searched for anything else.
		cel.Function(MatchAnyFunc, matchAnyOverloads("secl_match_any", celMatchAny)...),
		cel.Function(MatchAnyPathFunc, matchAnyOverloads("secl_match_any_path", celMatchAnyPath)...),
	}
}

// matchAnyOverloads declares one membership function, which is a function per semantics
// rather than a third argument so that a rule keeps to two arguments and the planner can
// recognise the call by name — see matchAnyDecorator.
//
// An overload id is unique across the whole environment rather than per function, which
// is what the prefix is for: the dispatcher refuses a second function declaring one it
// already knows.
func matchAnyOverloads(prefix string, binding functions.BinaryOp) []cel.FunctionOpt {
	return []cel.FunctionOpt{
		cel.Overload(prefix+"_patterns", []*cel.Type{cel.StringType, PatternsType}, cel.BoolType,
			cel.BinaryBinding(binding)),
		cel.Overload(prefix+"_strings", []*cel.Type{cel.StringType, cel.ListType(cel.StringType)}, cel.BoolType,
			cel.BinaryBinding(binding)),
		cel.Overload(prefix+"_ints", []*cel.Type{cel.IntType, cel.ListType(cel.IntType)}, cel.BoolType,
			cel.BinaryBinding(binding)),
		cel.Overload(prefix+"_bools", []*cel.Type{cel.BoolType, cel.ListType(cel.BoolType)}, cel.BoolType,
			cel.BinaryBinding(binding)),
	}
}

// matchAnyDecorator prepares the right hand side of a membership test when the rule
// is planned, whenever it is a value rather than something read from the event.
//
// It is what keeps a macro as cheap as a list written out. The translation cannot know
// what a name holds, so `x in SHELL_NAMES` becomes a MatchAnyFunc call over a declared
// constant — and left alone that would walk the list and compare boxed values per event,
// where cel-go's own `in` over a literal list folds into a hash lookup. Preparing the
// constant here gives the same lookup for a plain list and compiles each pattern once
// for a list holding some.
//
// It also removes the call: what is left is the subject, resolved through whatever the
// planner made of it, and one set lookup.
func matchAnyDecorator() interpreter.InterpretableDecoratorV2 {
	return func(i interpreter.InterpretableV2) (interpreter.InterpretableV2, error) {
		call, ok := i.(interpreter.InterpretableCall)
		if !ok || len(call.Args()) != 2 {
			return i, nil
		}

		var glob bool
		switch call.Function() {
		case MatchAnyFunc:
		case MatchAnyPathFunc:
			glob = true
		default:
			return i, nil
		}

		members, ok := call.Args()[1].(interpreter.InterpretableConst)
		if !ok {
			// The members are read from the event — a field reference — so there is
			// nothing to prepare and the call stands.
			return i, nil
		}
		set, ok := preparedSet(members.Value())
		if !ok {
			// A list of something other than strings, which the generic call handles
			// through the list's own Contains.
			return i, nil
		}
		// Which semantics the set is searched with is settled here, so a member that
		// cannot be matched with them is a rule that does not load rather than one
		// that errors per event.
		if err := set.supports(glob); err != nil {
			return nil, err
		}

		return &matchAnyIn{id: call.ID(), subject: call.Args()[0], set: set, glob: glob, generic: i}, nil
	}
}

// preparedSet is the set a constant right hand side is searched through, if it can be
// one: a set prepared by PatternsFunc already, or a list of plain strings.
func preparedSet(members ref.Val) (*patternSet, bool) {
	if set, ok := members.(*patternSet); ok {
		return set, true
	}
	prepared, err := newPatternSet(members)
	if err != nil {
		return nil, false
	}
	return prepared.(*patternSet), true
}

// matchAnyIn is a membership test against a set prepared at planning time.
type matchAnyIn struct {
	id      int64
	subject interpreter.InterpretableV2
	set     *patternSet
	// glob is the semantics the subject's field gives a pattern — see celGlobFields.
	glob bool

	// generic is the call as cel-go planned it, for a subject that turns out not to
	// be a string — an error or an unknown, which has to be propagated rather than
	// answered.
	generic interpreter.InterpretableV2
}

// ID implements interpreter.Interpretable.
func (m *matchAnyIn) ID() int64 { return m.id }

// Eval implements interpreter.Interpretable.
func (m *matchAnyIn) Eval(activation interpreter.Activation) ref.Val {
	return m.Exec(interpreter.AsFrame(activation))
}

// Exec implements interpreter.InterpretableV2.
func (m *matchAnyIn) Exec(frame *interpreter.ExecutionFrame) ref.Val {
	subject, ok := m.subject.Exec(frame).(types.String)
	if !ok {
		return m.generic.Exec(frame)
	}
	return types.Bool(m.set.contains(string(subject), m.glob))
}

func celGlobValue(pattern ref.Val) ref.Val {
	s, ok := pattern.(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(pattern)
	}
	value, err := newGlobValue(string(s))
	if err != nil {
		return types.WrapErr(err)
	}
	return value
}

func celRegexpValue(pattern ref.Val) ref.Val {
	s, ok := pattern.(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(pattern)
	}
	value, err := newRegexpValue(string(s))
	if err != nil {
		return types.WrapErr(err)
	}
	return value
}

func celPatterns(members ref.Val) ref.Val {
	set, err := newPatternSet(members)
	if err != nil {
		return types.WrapErr(err)
	}
	return set
}

// celMatchAny is SECL's `in` over anything a rule can name on the right of it: a prepared
// set of patterns, or an ordinary list.
//
// It is the fallback rather than the usual path: when the members are a value, the call
// is replaced at planning time — see matchAnyDecorator. What reaches it is a list read
// from the event, through a `%{…}` field reference.
func celMatchAny(subject, members ref.Val) ref.Val {
	return matchAny(subject, members, false)
}

// celMatchAnyPath is celMatchAny with the semantics a path field gives a pattern.
func celMatchAnyPath(subject, members ref.Val) ref.Val {
	return matchAny(subject, members, true)
}

func matchAny(subject, members ref.Val, glob bool) ref.Val {
	if set, ok := members.(*patternSet); ok {
		s, ok := subject.(types.String)
		if !ok {
			return types.MaybeNoSuchOverloadErr(subject)
		}
		if err := set.supports(glob); err != nil {
			return types.WrapErr(err)
		}
		return types.Bool(set.contains(string(s), glob))
	}

	list, ok := members.(traits.Container)
	if !ok {
		return types.MaybeNoSuchOverloadErr(members)
	}
	return list.Contains(subject)
}
