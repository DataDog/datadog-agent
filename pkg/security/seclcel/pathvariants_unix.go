// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// A path comparison in SECL reaches more than one value.
//
// The operator overrides a file field carries do not only choose a matcher — see
// celGlobFields — they also OR in comparisons against the *other* paths the same file is
// reachable by. `OverlayFSPathname` adds the path relative to an overlay mount point, and
// for `exec.file.path` and `process.file.path` `ProcessSymlinkPathname` adds the two
// symlink pathnames the resolver recorded; `ProcessSymlinkBasename` does the same for the
// basename of those two fields. So `open.file.path == "/etc/passwd"` matches a file opened
// through an overlay mount whose raw path is somewhere under /var/lib/docker.
//
// A rule that reads one value where SECL reads four is the narrower engine, and 234 of the
// 319 agent rules — 73% — mention a field with variants. So a comparison against such a
// field becomes one call over the paths it can take, with the other side of the comparison
// travelling as the matcher: PathMatchAnyFunc, which serves `==`, `=~` and `in` alike.
//
// A quantifier over the paths was the first shape tried, since the array semantics already
// express "some value matches". It cost seven times what the comparison it replaced did —
// a comprehension and the list it folds are several hundred nanoseconds, where the whole
// comparison is two hundred — so the loop is written out here instead and the paths are
// visited rather than collected. A path comparison now costs what it costs SECL.
//
// The values are computed here rather than read from the model, because the model's own
// evaluators are unexported closures (oo_overlayfs_unix.go, oo_symlink_unix.go).
// TestPathVariantsAgreeWithSECL is what catches the two drifting apart.

// symlinkPathFields and symlinkNameFields are the fields the symlink overrides act on.
// The overrides test the field name themselves — "currently only override exec events" —
// so this is that test, in the same order.
var (
	symlinkPathFields = map[string]bool{"exec.file.path": true, "process.file.path": true}
	symlinkNameFields = map[string]bool{"exec.file.name": true, "process.file.name": true}
)

// pathVariantReaders is what a field's variants are read by, keyed by field name and built
// once, since which variants a field has is a property of the model.
var pathVariantReaders = buildPathVariantReaders()

// buildPathVariantReaders asks the model which fields have variants.
//
// The overlay variant needs the file event behind the field, which the model exposes for a
// fixed set of names (Event.GetFileField) — an iterated field such as
// `process.ancestors.file.path` is not one of them, and SECL's own override yields an empty
// string and an error there rather than a path. Asking rather than listing is what keeps
// this in step with the model as file fields are added.
func buildPathVariantReaders() map[string]pathVariantReader {
	readers := map[string]pathVariantReader{}
	probe := model.NewFakeEvent()

	for field := range celReaderIndex {
		primary, ok := primaryReader(field)
		if !ok {
			continue
		}

		var alternates []func(*eval.Context) string

		if fileField, isPath := strings.CutSuffix(field, ".path"); isPath {
			if probe.ValidateFileField(fileField) == nil {
				alternates = append(alternates, overlayPath(fileField))
			}
		}
		if symlinkPathFields[field] {
			alternates = append(alternates, symlinkPath(0), symlinkPath(1))
		}
		if symlinkNameFields[field] {
			alternates = append(alternates, symlinkBasename)
		}

		if len(alternates) == 0 {
			continue
		}
		readers[field] = variantsReader(primary, alternates)
	}
	return readers
}

// primaryReader is the value the field itself reads, taken from the generated readers so
// that the variants of forty-odd fields do not restate forty-odd accessors.
func primaryReader(field string) (func(*eval.Context) string, bool) {
	index, ok := celReaderIndex[field]
	if !ok {
		return nil, false
	}

	read := celReaders[index]
	return func(ctx *eval.Context) string {
		value, ok := read(ctx, nil).(types.String)
		if !ok {
			// Not a string field, so it has no path variants.
			return ""
		}
		return string(value)
	}, true
}

// pathVariantReader hands each path a field can be compared against to visit, stopping at
// the first one visit accepts and reporting whether it did.
//
// It visits rather than returns a slice so that a comparison allocates nothing: the paths
// are read into a fixed array on the stack, and a match stops the read. A slice would be
// two allocations on every path comparison of every rule, which is the most common
// comparison the rule set makes.
type pathVariantReader func(ctx *eval.Context, visit func(path string) bool) bool

// maxPathVariants is the most paths a field can be compared against: its own value, the
// overlay path, and the two symlink pathnames.
const maxPathVariants = 4

// variantsReader visits the primary value and every alternate that says something the
// primary does not.
//
// SECL's own alternates fall back to the primary value when they are unset, so its
// disjunction compares the same string up to four times — its comment says as much. Here
// the duplicates are skipped instead, which is the same answer for less work: the common
// case, a file that is neither a symlink nor on an overlay mount, is one value.
func variantsReader(primary func(*eval.Context) string, alternates []func(*eval.Context) string) pathVariantReader {
	return func(ctx *eval.Context, visit func(path string) bool) bool {
		var seen [maxPathVariants]string

		value := primary(ctx)
		seen[0] = value
		count := 1
		if visit(value) {
			return true
		}

		for _, alternate := range alternates {
			value = alternate(ctx)
			if value == "" || contains(seen[:count], value) {
				continue
			}
			if count < len(seen) {
				seen[count] = value
				count++
			}
			if visit(value) {
				return true
			}
		}
		return false
	}
}

// overlayPath is the path relative to the overlay mount point the file was reached
// through, mirroring model.OverlayFSPathname's evaluator.
func overlayPath(fileField string) func(*eval.Context) string {
	return func(ctx *eval.Context) string {
		event, ok := ctx.Event.(*model.Event)
		if !ok {
			return ""
		}

		fileEvent, err := event.GetFileField(fileField)
		if err != nil {
			// The model refuses the field for this event — no interpreter, a kworker —
			// where its own evaluator records the error on the event. Reading nothing
			// here says the same thing without the side effect.
			return ""
		}

		if event.FieldHandlers.ResolveFileFilesystem(event, fileEvent) != "overlay" {
			return ""
		}
		path := event.FieldHandlers.ResolveFilePath(event, fileEvent)
		return strings.TrimPrefix(path, strings.TrimRight(fileEvent.MountPath, "/"))
	}
}

// symlinkPath is one of the two symlink pathnames the resolver recorded for the process,
// mirroring model.ProcessSymlinkPathname's evaluators.
func symlinkPath(slot int) func(*eval.Context) string {
	return func(ctx *eval.Context) string {
		event, ok := ctx.Event.(*model.Event)
		if !ok {
			return ""
		}
		return event.ProcessContext.SymlinkPathnameStr[slot]
	}
}

// symlinkBasename mirrors model.ProcessSymlinkBasename's evaluator.
func symlinkBasename(ctx *eval.Context) string {
	event, ok := ctx.Event.(*model.Event)
	if !ok {
		return ""
	}
	return event.ProcessContext.SymlinkBasenameStr
}

func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

// pathVariantBindings declares PathMatchAnyFunc, whose overloads are what keeps a path
// comparison type-checked: the matcher is one of the four shapes the other side of a
// comparison can be, and nothing else can be compared against a path.
func pathVariantBindings() []cel.EnvOption {
	overload := func(id string, matcher *cel.Type) cel.FunctionOpt {
		return cel.Overload(id, []*cel.Type{cel.DynType, cel.StringType, matcher}, cel.BoolType,
			cel.FunctionBinding(celPathMatchAny))
	}

	return []cel.EnvOption{
		cel.Function(PathMatchAnyFunc,
			// `path == x`, where x is a literal or anything else that reads as a string
			overload("secl_path_match_string", cel.StringType),
			// `path =~ "p"` and `path == ~"p"`
			overload("secl_path_match_glob", GlobType),
			// `path == r"re"`
			overload("secl_path_match_regexp", RegexpType),
			// `path in [ … ]`, prepared
			overload("secl_path_match_patterns", PatternsType),
			// and `path in MACRO`, where the macro is a plain list of strings
			overload("secl_path_match_strings", cel.ListType(cel.StringType))),
	}
}

// celPathMatchAny is the form a comparison takes only until the rule is planned: the field
// is a literal, so pathMatchDecorator resolves it to its reader and binds a matcher that
// does not depend on the event. What reaches this is a matcher that does — a comparison
// against another field, or against a variable.
func celPathMatchAny(args ...ref.Val) ref.Val {
	if len(args) != 3 {
		return types.NoSuchOverloadErr()
	}

	field, ok := args[1].(types.String)
	if !ok {
		return types.MaybeNoSuchOverloadErr(args[1])
	}
	read, ok := pathVariantReaders[string(field)]
	if !ok {
		return types.NewErr("%s has no path variants", field)
	}
	from, ok := args[0].(*seclPosition)
	if !ok {
		return types.NewErr("%s: %s is not a SECL event position", field, args[0].Type())
	}

	return matchesAnyPath(read, from.ctx, args[2], globPatterns(string(field)))
}

// matchesAnyPath is the disjunction SECL builds: does any of the paths the field can be
// compared against satisfy the matcher.
//
// Which shapes a matcher can take is settled by the overloads above, so an unexpected one
// is a translation that should not have been emitted rather than a rule that can fail.
func matchesAnyPath(read pathVariantReader, ctx *eval.Context, matcher ref.Val, glob bool) ref.Val {
	var test func(path string) bool

	switch matcher := matcher.(type) {
	case types.String:
		test = func(path string) bool { return path == string(matcher) }

	case patternMatcher:
		if err := matcher.supports(glob); err != nil {
			return types.WrapErr(err)
		}
		test = func(path string) bool { return matcher.matches(path, glob) }

	case *patternSet:
		if err := matcher.supports(glob); err != nil {
			return types.WrapErr(err)
		}
		test = func(path string) bool { return matcher.contains(path, glob) }

	case traits.Lister:
		test = func(path string) bool { return matcher.Contains(types.String(path)) == types.True }

	default:
		return types.MaybeNoSuchOverloadErr(matcher)
	}

	return types.Bool(read(ctx, test))
}

// globPatterns reports whether the field's patterns are globs, which the matcher needs and
// the field name decides — see celGlobFields.
func globPatterns(field string) bool {
	_, ok := celGlobFields[field]
	return ok
}

// pathMatchDecorator resolves the field and prepares the matcher when the rule is planned,
// which is where the cost of this shape goes: what is left per event is reading the paths
// and a loop over at most four strings.
func pathMatchDecorator() interpreter.InterpretableDecoratorV2 {
	return func(i interpreter.InterpretableV2) (interpreter.InterpretableV2, error) {
		call, ok := i.(interpreter.InterpretableCall)
		if !ok || call.Function() != PathMatchAnyFunc || len(call.Args()) != 3 {
			return i, nil
		}

		field, ok := call.Args()[1].(interpreter.InterpretableConst)
		if !ok {
			return i, nil
		}
		name, ok := field.Value().(types.String)
		if !ok {
			return i, nil
		}
		read, ok := pathVariantReaders[string(name)]
		if !ok {
			// A field the translation should not have asked for, which is a bug here
			// rather than a rule that cannot work.
			return nil, fmt.Errorf("%w: %s has no path variants", errUnsupportedValue, name)
		}

		variable, ok := positionVariable(call)
		if !ok {
			return i, nil
		}

		bound := &pathMatch{
			id:       call.ID(),
			variable: variable,
			read:     read,
			glob:     globPatterns(string(name)),
			matcher:  call.Args()[2],
			generic:  i,
		}

		// A matcher that is a value rather than something read from the event is
		// prepared now, and a list becomes the set it is searched through.
		if constant, isConst := bound.matcher.(interpreter.InterpretableConst); isConst {
			bound.value = constant.Value()
			if set, preparable := preparedSet(bound.value); preparable {
				bound.value = set
			}
			// Whether the matcher can be applied with these semantics is settled here
			// too, so a `**` pattern on a field that globs is a rule that does not load.
			if matcher, isMatcher := bound.value.(interface{ supports(bool) error }); isMatcher {
				if err := matcher.supports(bound.glob); err != nil {
					return nil, err
				}
			}
		}

		return bound, nil
	}
}

// pathMatch is a comparison against every path a field can be compared against, with the
// field's reader and the matcher bound.
type pathMatch struct {
	id       int64
	variable string
	read     pathVariantReader
	glob     bool

	// value is the matcher when it is known at planning time, and matcher the
	// interpretable to evaluate when it is not — a comparison against another field.
	value   ref.Val
	matcher interpreter.InterpretableV2

	// generic is the call as cel-go planned it, for an activation that hands out
	// something other than a SECL position.
	generic interpreter.InterpretableV2
}

// ID implements interpreter.Interpretable.
func (p *pathMatch) ID() int64 { return p.id }

// Eval implements interpreter.Interpretable.
func (p *pathMatch) Eval(activation interpreter.Activation) ref.Val {
	return p.Exec(interpreter.AsFrame(activation))
}

// Exec implements interpreter.InterpretableV2.
func (p *pathMatch) Exec(frame *interpreter.ExecutionFrame) ref.Val {
	value, found := frame.ResolveName(p.variable)
	position, ok := value.(*seclPosition)
	if !found || !ok {
		return p.generic.Exec(frame)
	}

	matcher := p.value
	if matcher == nil {
		matcher = p.matcher.Exec(frame)
	}
	return matchesAnyPath(p.read, position.ctx, matcher, p.glob)
}
