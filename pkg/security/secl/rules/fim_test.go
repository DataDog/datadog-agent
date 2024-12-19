// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package rules holds rules related files
package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandFIM(t *testing.T) {
	entries := []struct {
		id       string
		expr     string
		expected []expandedRule
	}{
		{
			id:   "test",
			expr: "fim.write.file.path == \"/tmp/test\"",
			expected: []expandedRule{
				{
					id:   "__fim_expanded_open_test",
					expr: "(open.file.path == \"/tmp/test\") && open.flags & (O_CREAT|O_TRUNC|O_APPEND|O_RDWR|O_WRONLY) > 0",
				},
				{
					id:   "__fim_expanded_chmod_test",
					expr: "chmod.file.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_chown_test",
					expr: "chown.file.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_link_test",
					expr: "link.file.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_rename_test",
					expr: "rename.file.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_rename_destination_test",
					expr: "rename.file.destination.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_unlink_test",
					expr: "unlink.file.path == \"/tmp/test\"",
				},
				{
					id:   "__fim_expanded_utimes_test",
					expr: "utimes.file.path == \"/tmp/test\"",
				},
			},
		},
		{
			id:   "complex",
			expr: "(fim.write.file.path == \"/tmp/test\" || fim.write.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
			expected: []expandedRule{
				{
					id:   "__fim_expanded_open_complex",
					expr: "((open.file.path == \"/tmp/test\" || open.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\") && open.flags & (O_CREAT|O_TRUNC|O_APPEND|O_RDWR|O_WRONLY) > 0",
				},
				{
					id:   "__fim_expanded_chmod_complex",
					expr: "(chmod.file.path == \"/tmp/test\" || chmod.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_chown_complex",
					expr: "(chown.file.path == \"/tmp/test\" || chown.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_link_complex",
					expr: "(link.file.path == \"/tmp/test\" || link.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_rename_complex",
					expr: "(rename.file.path == \"/tmp/test\" || rename.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_rename_destination_complex",
					expr: "(rename.file.destination.path == \"/tmp/test\" || rename.file.destination.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_unlink_complex",
					expr: "(unlink.file.path == \"/tmp/test\" || unlink.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
				{
					id:   "__fim_expanded_utimes_complex",
					expr: "(utimes.file.path == \"/tmp/test\" || utimes.file.name == \"abc\") && process.file.name == \"def\" && container.id != \"\"",
				},
			},
		},
	}

	for _, entry := range entries {
		t.Run(entry.id, func(t *testing.T) {
			actual := expandFim(entry.id, entry.expr)
			assert.Equal(t, entry.expected, actual)
		})
	}
}
