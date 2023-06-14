// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"

	"github.com/stretchr/testify/assert"
)

func TestSECLRuleFilter(t *testing.T) {
	kv := &kernel.Version{
		OsRelease:    map[string]string{},
		UnameRelease: "5.9.0-48-generic",
		Code:         kernel.Kernel5_9,
	}

	m := module.NewRuleFilterModel()
	m.Version = kv
	seclRuleFilter := rules.NewSECLRuleFilter(m)

	t.Run("true", func(t *testing.T) {
		result, err := seclRuleFilter.IsRuleAccepted(
			&rules.RuleDefinition{
				Filters: []string{
					"true",
				},
			},
		)
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("kernel-version", func(t *testing.T) {
		result, err := seclRuleFilter.IsRuleAccepted(
			&rules.RuleDefinition{
				Filters: []string{
					"kernel.version.major > 3",
				},
			},
		)
		assert.NoError(t, err)
		if runtime.GOOS == "windows" {
			assert.False(t, result)
		} else {
			assert.True(t, result)
		}
	})

	for _, os := range []string{"windows", "linux"} {
		t.Run("os-"+os, func(t *testing.T) {
			result, err := seclRuleFilter.IsRuleAccepted(
				&rules.RuleDefinition{
					Filters: []string{
						fmt.Sprintf(`os != "" && os == "%s"`, os),
					},
				},
			)
			assert.NoError(t, err)
			if runtime.GOOS == os {
				assert.True(t, result)
			} else {
				assert.False(t, result)
			}
		})
	}
}
