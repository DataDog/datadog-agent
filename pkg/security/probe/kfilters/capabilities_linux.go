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

// validateScalarPathFilter validates that the path can be handled by the basename filter
func validateScalarPathFilter(value rules.FilterValue) bool {
	switch value.Type {
	case eval.ScalarValueType:
		return true
	case eval.GlobValueType, eval.PatternValueType:
		pattern := path.Base(value.Value.(string))
		if !strings.Contains(pattern, "*") {
			return true
		}
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
				ValidateFnc:  validateScalarPathFilter,
				FilterWeight: 300,
			},
			{
				Field:        event + "." + field + ".name",
				TypeBitmask:  eval.ScalarValueType,
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
}
