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

func validateBasenameFilter(value rules.FilterValue) bool {
	switch value.Type {
	case eval.ScalarValueType:
		return true
	case eval.GlobValueType:
		basename := path.Base(value.Value.(string))
		if !strings.Contains(basename, "*") {
			return true
		}
	}

	return false
}

func buildBasenameCapabilities(event string, fields ...string) rules.FieldCapabilities {
	var caps rules.FieldCapabilities

	for _, field := range fields {
		caps = append(caps, rules.FieldCapabilities{
			{
				Field:       event + "." + field + ".path",
				TypeBitmask: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
				ValidateFnc: validateBasenameFilter,
			},
			{
				Field:       event + "." + field + ".name",
				TypeBitmask: eval.ScalarValueType,
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
	allCapabilities["chmod"] = mergeCapabilities(buildBasenameCapabilities("chmod", "file"), processCapabilities)
	allCapabilities["chown"] = mergeCapabilities(buildBasenameCapabilities("chown", "file"), processCapabilities)
	allCapabilities["link"] = mergeCapabilities(buildBasenameCapabilities("link", "file", "file.destination"), processCapabilities)
	allCapabilities["mkdir"] = mergeCapabilities(buildBasenameCapabilities("mkdir", "file"), processCapabilities)
	allCapabilities["open"] = openCapabilities
	allCapabilities["rename"] = mergeCapabilities(buildBasenameCapabilities("rename", "file", "file.destination"), processCapabilities)
	allCapabilities["rmdir"] = mergeCapabilities(buildBasenameCapabilities("rmdir", "file"), processCapabilities)
	allCapabilities["unlink"] = mergeCapabilities(buildBasenameCapabilities("unlink", "file"), processCapabilities)
	allCapabilities["utimes"] = mergeCapabilities(buildBasenameCapabilities("utimes", "file"), processCapabilities)
	allCapabilities["mmap"] = mmapCapabilities
	allCapabilities["mprotect"] = mprotectCapabilities
	allCapabilities["splice"] = spliceCapabilities
	allCapabilities["chdir"] = mergeCapabilities(buildBasenameCapabilities("chdir", "file"), processCapabilities)
	allCapabilities["bpf"] = bpfCapabilities
}
