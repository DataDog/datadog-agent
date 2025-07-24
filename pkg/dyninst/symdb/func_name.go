// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"fmt"
	"regexp"
	"strings"
)

// Utilities for parsing Go function names.

var (
	// Regex for parsing a package name. It consumes:
	// - an optional package path (greedily up to the last slash)
	// - everything up to the following dot (lazily)
	// - the trailing dot, outside of the named capture
	pkgNameRegex = `^(?P<pkg>(.*/)?[^.]*?)\.`

	methodWithPtrReceiverRE = regexp.MustCompile(
		pkgNameRegex + `\(\*(?P<type>\w*)\)\.(?P<name>.*)$`)
	methodWithPtrReceiverREPkgIdx  = methodWithPtrReceiverRE.SubexpIndex("pkg")
	methodWithPtrReceiverRETypeIdx = methodWithPtrReceiverRE.SubexpIndex("type")
	methodWithPtrReceiverRENameIdx = methodWithPtrReceiverRE.SubexpIndex("name")

	methodWithValueReceiverRE = regexp.MustCompile(
		pkgNameRegex + `(?P<type>\w+)\.(?P<name>.*)$`)
	methodWithValueReceiverREPkgIdx  = methodWithValueReceiverRE.SubexpIndex("pkg")
	methodWithValueReceiverRETypeIdx = methodWithValueReceiverRE.SubexpIndex("type")
	methodWithValueReceiverRENameIdx = methodWithValueReceiverRE.SubexpIndex("name")

	anonymousFuncRE = regexp.MustCompile(`^(func)?\d+`)

	standaloneFuncRE = regexp.MustCompile(
		pkgNameRegex + `(?P<name>.*)$`)
	standaloneFuncREPkgIdx  = standaloneFuncRE.SubexpIndex("pkg")
	standaloneFuncRENameIdx = standaloneFuncRE.SubexpIndex("name")
)

type parseFuncNameFailureReason int

const (
	parseFuncNameFailureReasonUndefined parseFuncNameFailureReason = iota
	// parseFuncNameFailureReasonGenericFunction is used if the function takes
	// type arguments.
	parseFuncNameFailureReasonGenericFunction
	// Functions like time.map.init.0 that initialize statically-defined maps.
	parseFuncNameFailureReasonMapInit
)

// funcName is the result of parsing a Go function name by parseFuncName().
type funcName struct {
	Package string
	// Type is the type of the receiver, if any. Empty if this function is not a
	// method. The type is not a pointer type even if the method has a
	// pointer-receiver; the base type is returned, without the '*'.
	Type string
	Name string
	// QualifiedName looks like
	// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
	QualifiedName string
}

func (f *funcName) Empty() bool {
	return f.Name == ""
}

// parseFuncNameResult is the result of parsing a Go function name by
// parseFuncName().
type parseFuncNameResult struct {
	// failureReason is set if the function name was not be parsed because the
	// function is not supported. Such functions should be ignored.
	failureReason parseFuncNameFailureReason
	// funcName is the parsed function name. Set if failureReason is not set.
	funcName funcName
}

// parseFuncName parses a Go qualified function name. For a qualifiedName name
// like:
// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
// the package is: github.com/cockroachdb/cockroach/pkg/kv/kvserver
// the type is: raftSchedulerShard (note that it doesn't include the '*' signifying a pointer receiver).
// the name is: worker
//
// Some functions are not supported. For these, failureReason is set on the
// result and a nil error is returned. A returned error indicates an unexpected
// failure.
//
// Cases we need to support:
// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
// github.com/cockroachdb/cockroach/pkg/ccl/changefeedccl/kvfeed.rangefeedFactory.Run
// github.com/klauspost/compress/zstd.sequenceDecs_decode_amd64
// indexbytebody
// github.com/.../pkg.myFunc.func0
// internal/bytealg.init.0
//
// Cases we don't currently support, but we should:
// - Anonymous functions defined inside methods with value receivers, e.g.:
// github.com/cockroachdb/pebble/wal.FailoverOptions.EnsureDefaults.func1
// (we don't support these because we confuse them with anonymous functions
// called from inlined functions)
// - Nested anonymous functions, e.g.:
// github.com/cockroachdb/cockroach/pkg/server.(*apiV2Server).execSQL.func8.1.3.2
// (we don't support these because we also confuse them with anonymous functions
// called from inlined functions)
func parseFuncName(qualifiedName string) (parseFuncNameResult, error) {
	// Ignore generic functions, e.g.
	// os.init.OnceValue[go.shape.interface { Error() string }].func3
	// Note that this name is weird -- os.init is neither a package nor a type,
	// but rather it has something to do with the generic function's caller.
	if strings.ContainsRune(qualifiedName, '[') {
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonGenericFunction,
		}, nil
	}

	// Ignore map initialization functions like time.map.init.0. These initialize
	// global map variables.
	if strings.Contains(qualifiedName, ".map.init.") {
		return parseFuncNameResult{
			failureReason: parseFuncNameFailureReasonMapInit,
		}, nil
	}

	// Parse the function name as either a method on a pointer receiver, a
	// method on a value receiver, or a standalone function.
	var pkg, typ, name string
	groups := methodWithPtrReceiverRE.FindStringSubmatch(qualifiedName)
	if groups != nil {
		pkg = groups[methodWithPtrReceiverREPkgIdx]
		typ = groups[methodWithPtrReceiverRETypeIdx]
		name = groups[methodWithPtrReceiverRENameIdx]
	} else if groups = methodWithValueReceiverRE.FindStringSubmatch(qualifiedName); groups != nil {
		pkg = groups[methodWithValueReceiverREPkgIdx]
		typ = groups[methodWithValueReceiverRETypeIdx]
		name = groups[methodWithValueReceiverRENameIdx]

		// Disambiguate between two cases:
		// The following example function:
		// github.com/getsentry/sentry-go.NewClient.func1
		// could either be a method called func1 on a type called NewClient, or
		// an anonymous function defined inside a function called NewClient. We
		// recognize certain names as indicating anonymous functions.
		if anonymousFuncRE.MatchString(name) {
			name = typ + "." + name
			typ = ""
		}
	} else {
		// If the function is not a method, it should be a standalone function.
		groups = standaloneFuncRE.FindStringSubmatch(qualifiedName)
		if groups == nil {
			return parseFuncNameResult{}, fmt.Errorf("failed to parse function qualified name: %s", qualifiedName)
		}
		pkg = groups[standaloneFuncREPkgIdx]
		name = groups[standaloneFuncRENameIdx]
	}

	// Check whether we're with anonymous functions. If we are, they might have
	// parsed as a method, but they are not actually methods, so we need to
	// rectify the results of the parsing and wipe the type.
	cnt := strings.Count(name, ".")
	if cnt == 0 {
		// This is the straight-forward case; this is a standalone function or a
		// method, not an anonymous function.
		return parseFuncNameResult{
			funcName: funcName{
				Package:       pkg,
				Type:          typ,
				Name:          name,
				QualifiedName: qualifiedName,
			},
		}, nil
	}
	// There are two possibilities (including their more deeply nested cases):
	// 1. This is an anonymous function defined inside a function/method, like:
	// github.com/andrei/project/pkg/mypkg.myFunc.func1
	// github.com/andrei/project/pkg/mypkg.myType.myMethod.func1
	// 2. This is an instantiation of an anonymous function defined inside another function
	// that's called from a function that was inlined in our function.method, like:
	// github.com/andrei/project/pkg/mypkg.myFunc.anotherFunc.func1
	// github.com/andrei/project/pkg/mypkg.myType.myMethod.anotherFunc.func1
	//
	// In either case, this function is not a method.
	// TODO: We should try to distinguish the second case and ignore
	// these functions; a user shouldn't see them (ideally we'd
	// treat them as inlined instances of the respective anonymous
	// function, but unfortunately Go's DWARF does not provide a
	// link to the "real" function).

	finalName := name
	if typ != "" {
		finalName = typ + "." + name
	}
	return parseFuncNameResult{
		funcName: funcName{
			Package:       pkg,
			Type:          "",
			Name:          finalName,
			QualifiedName: qualifiedName,
		},
	}, nil
}
