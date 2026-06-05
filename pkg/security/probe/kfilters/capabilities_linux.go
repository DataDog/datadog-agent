// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	patternPrefixSize = 3 // has to be in sync with the kernel side of the approvers
	// minParentBasenameSize avoids registering a parent-basename approver on a very short directory name
	// (e.g. "a"), which would match files in any same-named directory across the filesystem and make the
	// kernel approver too coarse to filter anything useful.
	minParentBasenameSize = 3
)

func validateBasenameFilter(pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return true
	}

	// Only accept wildcard basenames with a pre-'*' prefix of at least
	// patternPrefixSize bytes: newBasenameKFilter slices els[0] to that
	// length without bounds-checking, and shorter prefixes would be too
	// coarse to be useful as kernel approvers anyway.
	els := strings.Split(pattern, "*")
	return len(els[0]) >= patternPrefixSize
}

func validateParentBasenameFilter(pattern string) bool {
	return !strings.Contains(pattern, "*")
}

// validatePathFilter validates that the path can be handled by the basename filter
func validatePathFilter(value rules.FilterValue) bool {
	switch value.Type {
	case eval.ScalarValueType:
		return true
	case eval.GlobValueType, eval.PatternValueType:
		if validateBasenameFilter(path.Base(value.Value.(string))) {
			return true
		}

		parentBasename := path.Base(path.Dir(value.Value.(string)))
		if len(parentBasename) < minParentBasenameSize {
			return false
		}

		return validateParentBasenameFilter(parentBasename)
	}

	return false
}

// validateNameFilter validates the name
func validateNameFilter(value rules.FilterValue) bool {
	switch value.Type {
	case eval.ScalarValueType:
		return true
	case eval.PatternValueType:
		return validateBasenameFilter(path.Base(value.Value.(string)))
	}

	return false
}

func buildFileCapabilities(event string, fields ...string) rules.FieldCapabilities {
	var caps rules.FieldCapabilities

	for _, field := range fields {
		caps = append(caps, rules.FieldCapabilities{
			{
				Field:        event + "." + field + ".path",
				TypeBitmask:  eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
				ValidateFnc:  validatePathFilter,
				FilterWeight: 300,
				// FilterWeightFnc adjusts the path approver weight depending on what can actually
				// be pushed to the kernel. A precise leaf-basename approver keeps the full weight,
				// while a coarse parent-basename approver is demoted below the flags approvers.
				FilterWeightFnc: func(value rules.FilterValue) int {
					strValue, ok := value.Value.(string)
					if !ok {
						// only string paths can be demoted to a parent-basename approver; keep the full weight
						return 300
					}
					// the leaf basename is a usable kernel approver (a plain basename, or a wildcard
					// with a prefix of at least patternPrefixSize bytes): keep the full weight
					if validateBasenameFilter(path.Base(strValue)) {
						return 300
					}
					// otherwise only the parent directory basename can be pushed to the kernel, which is
					// a coarse approver: weigh it below the flags approvers (open.flags is 100,
					// open.file.in_upper_layer is 50) so a more selective field is preferred when available
					return 20
				},
			},
			{
				Field:        event + "." + field + ".name",
				TypeBitmask:  eval.ScalarValueType,
				ValidateFnc:  validateNameFilter,
				FilterWeight: 300,
			},
		}...)
	}
	return caps
}

func mergeCapabilities(caps ...rules.FieldCapabilities) rules.FieldCapabilities {
	var result rules.FieldCapabilities
	for _, c := range caps {
		result = append(result, c...)
	}
	return result
}

func init() {
	allCapabilities["chmod"] = mergeCapabilities(buildFileCapabilities("chmod", "file"), processCapabilities)
	allCapabilities["chown"] = mergeCapabilities(buildFileCapabilities("chown", "file"), processCapabilities)
	allCapabilities["link"] = mergeCapabilities(buildFileCapabilities("link", "file", "file.destination"), processCapabilities)
	allCapabilities["mkdir"] = mergeCapabilities(buildFileCapabilities("mkdir", "file"), processCapabilities)
	allCapabilities["open"] = mergeCapabilities(buildFileCapabilities("open", "file"), openFlagsCapabilities, processCapabilities)
	allCapabilities["rename"] = mergeCapabilities(buildFileCapabilities("rename", "file", "file.destination"), processCapabilities)
	allCapabilities["rmdir"] = mergeCapabilities(buildFileCapabilities("rmdir", "file"), processCapabilities)
	allCapabilities["unlink"] = mergeCapabilities(buildFileCapabilities("unlink", "file"), processCapabilities)
	allCapabilities["utimes"] = mergeCapabilities(buildFileCapabilities("utimes", "file"), processCapabilities)
	allCapabilities["mmap"] = mergeCapabilities(buildFileCapabilities("mmap", "file"), mmapCapabilities, processCapabilities)
	allCapabilities["mprotect"] = mprotectCapabilities
	allCapabilities["splice"] = mergeCapabilities(buildFileCapabilities("splice", "file"), spliceCapabilities, processCapabilities)
	allCapabilities["chdir"] = mergeCapabilities(buildFileCapabilities("chdir", "file"), processCapabilities)
	allCapabilities["bpf"] = bpfCapabilities
	allCapabilities["sysctl"] = sysctlCapabilities
	allCapabilities["connect"] = connectCapabilities
	allCapabilities["prctl"] = prctlCapabilities
	allCapabilities["setsockopt"] = setsockoptCapabilities
	allCapabilities["socket"] = socketCapabilities
}
