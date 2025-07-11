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
	// The parsing goes as follows:
	// - an optional package name: (((.*/)?.*?)\.)?. Consume (greedily)
	// everything up to the last slash, and then non-greedily up to the
	// following dot.
	// - an optional type name: (\(?\*?(.*?)\)?\.)?. The type name maybe be
	// between parens and also start with a '*' if it's a pointer receiver;
	// otherwise the type name is not wrapped in parens. Note that we won't
	// parse correctly if there is a type name and no package name. Hopefully that
	// doesn't exist.
	//   - note that we don't include the optional '*' in the capture group. We
	//     don't differentiate between pointer and value receivers.
	//   - note the lazy capture group for the type name: .*?. It's lazy because
	//     otherwise it also captures the following ')'
	//   - the function name: (.*)
	parseFuncNameRE = regexp.MustCompile(`^((?P<pkg>(.*/)?.*?)\.)?(\(?\*?(?P<typ>.*?)\)?\.)?(?P<name>.*)$`)
	pkgIdx          = parseFuncNameRE.SubexpIndex("pkg")
	typIdx          = parseFuncNameRE.SubexpIndex("typ")
	nameIdx         = parseFuncNameRE.SubexpIndex("name")

	// Recognize anonymous functions declared inside a function that is not a method.
	// They look like:
	// github.com/.../pkg.myFunc.func1
	// internal/bytealg.init.0
	parseAnonymousFuncNameRE = regexp.MustCompile(`^((?P<pkg>(.*/)?\w*?)\.)?(?P<func>(\w*?\.)?((func|gowrap|(\d+)).*))$`)
	anonPkgIdx               = parseFuncNameRE.SubexpIndex("pkg")
	anonNameIdx              = parseFuncNameRE.SubexpIndex("name")

	// Recognize funky functions corresponding to anonymous functions defined
	// inside inlined functions. These have names like:
	// runtime.gcMarkDone.forEachP.func5
	// "forEachP.func5" is an anonymous function defined inside forEachP, and
	// "runtime.gcMarkDone" is the functions inside which "forEachP" is inlined.
	parseAnonFuncInsideInlinedFunc = regexp.MustCompile(`^((?P<pkg>(.*/)?\w*?)\.)\w+\.\w+\.func(\d)+$`)
)

type FuncName struct {
	Package string
	// Type is the type of the receiver, if any. Empty if this function is not a
	// method. The type is not a pointer type even if the method has a
	// pointer-receiver; the base type is returned, without the '*'.
	Type string
	Name string
	// QualifiedName looks like
	// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
	QualifiedName string

	// GenericFunction is set if the function takes type arguments. We don't
	// support parsing these functions at the moment, so no other fields except
	// QualifiedName are set.
	GenericFunction bool
}

func (f *FuncName) Empty() bool {
	return *f == (FuncName{})
}

// ParseFuncName parses a Go qualified function name. For a qualifiedName name
// like:
// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
// the package is: github.com/cockroachdb/cockroach/pkg/kv/kvserver
// the type is: raftSchedulerShard (note that it doesn't include the '*' signifying a pointer receiver).
// the name is: worker
//
// Returns (zero value, nil) if the function should be ignored.
//
// Cases we need to support:
// github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker
// github.com/cockroachdb/cockroach/pkg/ccl/changefeedccl/kvfeed.rangefeedFactory.Run
// github.com/klauspost/compress/zstd.sequenceDecs_decode_amd64
// indexbytebody
// github.com/.../pkg.myFunc.func0
// internal/bytealg.init.0
func ParseFuncName(qualifiedName string) (FuncName, error) {
	// Filter out generic functions, e.g.
	// os.init.OnceValue[go.shape.interface { Error() string }].func3
	// Note that this name is weird -- os.init is neither a package nor a type,
	// but rather it has something to do with the generic function's caller.
	if strings.ContainsRune(qualifiedName, '[') {
		return FuncName{
			QualifiedName:   qualifiedName,
			GenericFunction: true,
		}, nil
	}

	// Ignore anonymous functions declared inside inlined functions. These are
	// DWARF entries corresponding to anonymous functions that are defined
	// inside a function that was inlined. We don't know what to do with them
	// because the debug info doesn't point back to the abstract origin.
	if parseAnonFuncInsideInlinedFunc.FindString(qualifiedName) != "" {
		return FuncName{}, nil
	}

	// First, we need to distinguish between the following cases:
	// github.com/.../pkg.myType.myMethod
	// and
	// github.com/.../pkg.myFunc.func1
	//
	// The former is a method on a type called myType. The latter is a function
	// called myFunc.func1. We recognize the former case by checking if the name
	// starts with some known prefixes. Note that a method like:
	// github.com/.../pkg.myType.myMethod.func1
	// will not match our regex; it'll be handled by the general case below.

	// See if the function name looks like an anonymous function declared inside a
	// function that's not a method.
	groups := parseAnonymousFuncNameRE.FindStringSubmatch(qualifiedName)
	if groups != nil {
		return FuncName{
			Package:       groups[anonPkgIdx],
			Name:          groups[anonNameIdx],
			QualifiedName: qualifiedName,
		}, nil
	}

	// We're not dealing with the anonymous function in function case.
	groups = parseFuncNameRE.FindStringSubmatch(qualifiedName)
	if groups == nil {
		return FuncName{}, fmt.Errorf("failed to parse function qualified name: %s", qualifiedName)
	}

	return FuncName{
		Package:       groups[pkgIdx],
		Type:          groups[typIdx],
		Name:          groups[nameIdx],
		QualifiedName: qualifiedName,
	}, nil
}
