// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

// This file contains a copy of the regex-based symbol parser from
// pkg/dyninst/symdb/func_name.go for benchmark comparison purposes.
// The original has a linux_bpf build tag, so we copy it here to run
// on all platforms.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var (
	pkgNameRegex = `^(?P<pkg>(.*/)?[^.]*?)\.`

	oldMethodWithPtrReceiverRE        = regexp.MustCompile(pkgNameRegex + `\(\*(?P<type>\w*)\)\.(?P<name>.*)$`)
	oldMethodWithPtrReceiverREPkgIdx  = oldMethodWithPtrReceiverRE.SubexpIndex("pkg")
	oldMethodWithPtrReceiverRETypeIdx = oldMethodWithPtrReceiverRE.SubexpIndex("type")
	oldMethodWithPtrReceiverRENameIdx = oldMethodWithPtrReceiverRE.SubexpIndex("name")

	oldMethodWithValueReceiverRE        = regexp.MustCompile(pkgNameRegex + `(?P<type>\w+)\.(?P<name>.*)$`)
	oldMethodWithValueReceiverREPkgIdx  = oldMethodWithValueReceiverRE.SubexpIndex("pkg")
	oldMethodWithValueReceiverRETypeIdx = oldMethodWithValueReceiverRE.SubexpIndex("type")
	oldMethodWithValueReceiverRENameIdx = oldMethodWithValueReceiverRE.SubexpIndex("name")

	oldAnonymousFuncRE = regexp.MustCompile(`(^(func)?\d+)|(-range\d+$)`)

	oldStandaloneFuncRE        = regexp.MustCompile(pkgNameRegex + `(?P<name>.*)$`)
	oldStandaloneFuncREPkgIdx  = oldStandaloneFuncRE.SubexpIndex("pkg")
	oldStandaloneFuncRENameIdx = oldStandaloneFuncRE.SubexpIndex("name")
)

type oldFuncName struct {
	Package       string
	Type          string
	Name          string
	QualifiedName string
}

// oldParseFuncName is a copy of symdb.parseFuncName for benchmarking.
func oldParseFuncName(qualifiedName string) (oldFuncName, error) {
	if strings.ContainsRune(qualifiedName, '[') {
		return oldFuncName{}, errors.New("generic function")
	}
	if strings.Contains(qualifiedName, ".map.init.") {
		return oldFuncName{}, errors.New("map init")
	}

	var pkg, typ, name string
	groups := oldMethodWithPtrReceiverRE.FindStringSubmatch(qualifiedName)
	if groups != nil {
		pkg = groups[oldMethodWithPtrReceiverREPkgIdx]
		typ = groups[oldMethodWithPtrReceiverRETypeIdx]
		name = groups[oldMethodWithPtrReceiverRENameIdx]
	} else if groups = oldMethodWithValueReceiverRE.FindStringSubmatch(qualifiedName); groups != nil {
		pkg = groups[oldMethodWithValueReceiverREPkgIdx]
		typ = groups[oldMethodWithValueReceiverRETypeIdx]
		name = groups[oldMethodWithValueReceiverRENameIdx]
	} else {
		groups = oldStandaloneFuncRE.FindStringSubmatch(qualifiedName)
		if groups == nil {
			return oldFuncName{}, fmt.Errorf("failed to parse: %s", qualifiedName)
		}
		pkg = groups[oldStandaloneFuncREPkgIdx]
		name = groups[oldStandaloneFuncRENameIdx]
	}

	pkg = strings.ReplaceAll(pkg, "%2e", ".")

	cnt := strings.Count(name, ".")
	if cnt == 0 {
		return oldFuncName{
			Package:       pkg,
			Type:          typ,
			Name:          name,
			QualifiedName: qualifiedName,
		}, nil
	}

	if oldAnonymousFuncRE.MatchString(name) && typ != "" {
		name = typ + "." + name
		typ = ""
	}

	finalName := name
	if typ != "" {
		finalName = typ + "." + name
	}
	return oldFuncName{
		Package:       pkg,
		Type:          "",
		Name:          finalName,
		QualifiedName: qualifiedName,
	}, nil
}

// BenchmarkOldRegexParse benchmarks the old regex-based parser from symdb
// on the same inputs as the other benchmarks for comparison.
func BenchmarkOldRegexParse(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = oldParseFuncName(bs.input)
			}
		})
	}
}
