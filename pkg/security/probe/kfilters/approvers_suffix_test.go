// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func openApprovers(t *testing.T, exprs ...string) rules.Approvers {
	t.Helper()

	ruleOpts, evalOpts := rules.NewBothOpts(map[eval.EventType]bool{"*": true})
	rs := rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	rules.AddTestRuleExpr(t, rs, exprs...)

	approvers, _, _, err := rs.GetEventTypeApprovers("open", models["open"])
	require.NoError(t, err)
	return approvers
}

// basenameKeys returns the basename map keys the open kfilters getter would push to the kernel.
func basenameKeys(t *testing.T, approvers rules.Approvers) []basenameApprover {
	t.Helper()

	kfilters, err := NewKFilters("open", approvers)
	require.NoError(t, err)

	var keys []basenameApprover
	for _, kfilter := range kfilters {
		if f, ok := kfilter.(*eventMaskKFilter); ok {
			if key, ok := f.tableKey.(*basenameApprover); ok {
				keys = append(keys, *key)
			}
		}
	}
	return keys
}

func TestApproverBasenameSuffix(t *testing.T) {
	t.Run("name-suffix-wins-over-flags", func(t *testing.T) {
		approvers := openApprovers(t, `open.file.name =~ "*.php" && (open.flags & O_ACCMODE == O_RDONLY || open.flags & O_ACCMODE == O_RDWR)`)

		assert.NotContains(t, approvers, "open.flags")
		assert.Equal(t, []basenameApprover{{kind: leafBasenameSuffix, value: "php"}}, basenameKeys(t, approvers))
	})

	t.Run("path-leaf-suffix", func(t *testing.T) {
		approvers := openApprovers(t, `open.file.path =~ "/etc/systemd/system/*.service" && open.flags & O_CREAT > 0`)

		assert.NotContains(t, approvers, "open.flags")
		assert.Equal(t, []basenameApprover{{kind: leafBasenameSuffix, value: "ice"}}, basenameKeys(t, approvers))
	})

	t.Run("prefix-still-preferred", func(t *testing.T) {
		approvers := openApprovers(t, `open.file.name =~ "libdd*.so" && open.flags & O_CREAT > 0`)

		assert.Equal(t, []basenameApprover{{kind: leafBasenamePrefix, value: "lib"}}, basenameKeys(t, approvers))
	})

	t.Run("short-suffix-falls-back-to-flags", func(t *testing.T) {
		approvers := openApprovers(t, `open.file.path =~ "/home/*/*rc" && open.flags & O_CREAT > 0`)

		assert.NotContains(t, approvers, "open.file.path")
		assert.Contains(t, approvers, "open.flags")
	})

	t.Run("suffix-after-last-star", func(t *testing.T) {
		approvers := openApprovers(t, `open.file.name =~ "ab*.php" && open.flags & O_CREAT > 0`)

		assert.NotContains(t, approvers, "open.flags")
		assert.Equal(t, []basenameApprover{{kind: leafBasenameSuffix, value: "php"}}, basenameKeys(t, approvers))
	})

	t.Run("suffix-pattern", func(t *testing.T) {
		assert.True(t, isSuffixPattern("*.php"))
		assert.True(t, isSuffixPattern("*foo*.php"))
		assert.False(t, isSuffixPattern("**"))
		assert.False(t, isSuffixPattern("*.p*p"))
		assert.False(t, isSuffixPattern(".php"))
	})

	// php_uploaded_script_read as shipped in threat-detection.policy (variable clause removed): without
	// the suffix approver its only approver is the access mode, a scalar O_RDONLY that turns on the
	// read-only flags approver and lets every open through.
	t.Run("php_uploaded_script_read", func(t *testing.T) {
		approvers := openApprovers(t, `open.retval >= 0 && open.file.name =~ "*.php" && open.file.mount_id != 0 && open.file.inode != 0 &&
(open.flags & O_ACCMODE == O_RDONLY || open.flags & O_ACCMODE == O_RDWR) &&
open.flags & O_PATH == 0 &&
process.file.name in [~"php*", ~"lsphp*", "apache2", "httpd", "frankenphp"]`)

		assert.NotContains(t, approvers, "open.flags")
		assert.Equal(t, []basenameApprover{{kind: leafBasenameSuffix, value: "php"}}, basenameKeys(t, approvers))
	})
}
