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
	parseAnonymousFuncNameRE = regexp.MustCompile(`^((?P<pkg>(.*/)?[^(]*?)\.)?(?P<func>(\w*?\.)?((func|gowrap|(\d+)).*))$`)
	anonPkgIdx               = parseAnonymousFuncNameRE.SubexpIndex("pkg")
	anonNameIdx              = parseAnonymousFuncNameRE.SubexpIndex("func")

	// Recognize funky functions corresponding to anonymous functions defined
	// inside inlined functions. These have names like:
	// runtime.gcMarkDone.forEachP.func5
	// "forEachP.func5" is an anonymous function defined inside forEachP, and
	// "runtime.gcMarkDone" is the function inside which "forEachP" is inlined.
	//
	// Note that we also have anonymous methods inside inlined methods, like:
	// github.com/cockroachdb/cockroach/pkg/server.(*topLevelServer).startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1
	// The regex does not match these; they are recognized in code.
	parseAnonFuncInsideInlinedFuncRE = regexp.MustCompile(`^((?P<pkg>(.*/)?.*?)\.)\w+(\.\w+)+\.(func)?(\d)+$`)
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

	// GenericFunction is set if the function takes type arguments. We don't
	// support parsing these functions at the moment, so no other fields except
	// QualifiedName are set.
	GenericFunction bool
}

func (f *funcName) Empty() bool {
	return *f == (funcName{})
}

// parseFuncName parses a Go qualified function name. For a qualifiedName name
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
//
// Cases we don't currently support, but we should:
// - Anonymous functions defined inside methods:
// github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle.preferFollowerOracle.(*ChoosePreferredReplica).func1
// github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle.preferFollowerOracle.ChoosePreferredReplica.func1
// (we don't support these because we confuse them with anonymous functions called from inlined functions)
func parseFuncName(qualifiedName string) (funcName, error) {
	// Filter out generic functions, e.g.
	// os.init.OnceValue[go.shape.interface { Error() string }].func3
	// Note that this name is weird -- os.init is neither a package nor a type,
	// but rather it has something to do with the generic function's caller.
	if strings.ContainsRune(qualifiedName, '[') {
		return funcName{
			QualifiedName:   qualifiedName,
			GenericFunction: true,
		}, nil
	}

	// Ignore map initialization functions like time.map.init.0. These initialize
	// global map variables.
	if strings.Contains(qualifiedName, ".map.init.") {
		return funcName{}, nil
	}

	// Ignore anonymous functions declared inside inlined functions. These are
	// DWARF entries corresponding to anonymous functions that are defined
	// inside a function that was inlined. We don't know what to do with them
	// because the debug info doesn't point back to the abstract origin.
	if parseAnonFuncInsideInlinedFuncRE.FindString(qualifiedName) != "" {
		return funcName{}, nil
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
		return funcName{
			Package:       groups[anonPkgIdx],
			Name:          groups[anonNameIdx],
			QualifiedName: qualifiedName,
		}, nil
	}

	// We're done with the special cases. Now parse the general case.
	groups = parseFuncNameRE.FindStringSubmatch(qualifiedName)
	if groups == nil {
		return funcName{}, fmt.Errorf("failed to parse function qualified name: %s", qualifiedName)
	}

	// Ignore funky functions like:
	// github.com/cockroachdb/cockroach/pkg/server.(*topLevelServer).startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1
	// This is an anonymous function ((*Node).SetHLCUpperBound.1) that is called from an inlined instance
	// of github.com/cockroachdb/cockroach/pkg/server.(*Node).SetHLCUpperBound (inlined inside
	// github.com/cockroachdb/cockroach/pkg/server.(*topLevelServer).startPersistingHLCUpperBound.func1).
	// This is similar to the case handled above by parseAnonFuncInsideInlinedFuncRE.
	// That function is parsed above as having name:
	// startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1.
	// We recognize the case by the parens around (*Node). Note that we fail to
	// recognize such functions when their receiver is not a pointer.
	name := groups[nameIdx]
	if strings.ContainsRune(name, '(') {
		return funcName{}, nil
	}

	return funcName{
		Package:       groups[pkgIdx],
		Type:          groups[typIdx],
		Name:          groups[nameIdx],
		QualifiedName: qualifiedName,
	}, nil
}
