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

func oneBasenameCapabilities(event string) rules.FieldCapabilities {
	return rules.FieldCapabilities{
		{
			Field:       event + ".file.path",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType | eval.GlobValueType,
			ValidateFnc: validateBasenameFilter,
		},
		{
			Field:       event + ".file.name",
			TypeBitmask: eval.ScalarValueType,
		},
	}
}

func twoBasenameCapabilities(event string, field1, field2 string) rules.FieldCapabilities {
	return rules.FieldCapabilities{
		{
			Field:       event + "." + field1 + ".path",
			TypeBitmask: eval.ScalarValueType | eval.GlobValueType,
			ValidateFnc: validateBasenameFilter,
		},
		{
			Field:       event + "." + field1 + ".name",
			TypeBitmask: eval.ScalarValueType,
		},
		{
			Field:       event + "." + field2 + ".path",
			TypeBitmask: eval.ScalarValueType | eval.GlobValueType,
			ValidateFnc: validateBasenameFilter,
		},
		{
			Field:       event + "." + field2 + ".name",
			TypeBitmask: eval.ScalarValueType,
		},
	}
}

func init() {
	allCapabilities["chmod"] = oneBasenameCapabilities("chmod")
	allCapabilities["chown"] = oneBasenameCapabilities("chown")
	allCapabilities["link"] = twoBasenameCapabilities("link", "file", "file.destination")
	allCapabilities["mkdir"] = oneBasenameCapabilities("mkdir")
	allCapabilities["open"] = openCapabilities
	allCapabilities["rename"] = twoBasenameCapabilities("rename", "file", "file.destination")
	allCapabilities["rmdir"] = oneBasenameCapabilities("rmdir")
	allCapabilities["unlink"] = oneBasenameCapabilities("unlink")
	allCapabilities["utimes"] = oneBasenameCapabilities("utimes")
	allCapabilities["mmap"] = mmapCapabilities
	allCapabilities["mprotect"] = mprotectCapabilities
	allCapabilities["splice"] = spliceCapabilities
	allCapabilities["chdir"] = oneBasenameCapabilities("chdir")
	allCapabilities["bpf"] = bpfCapabilities
}
