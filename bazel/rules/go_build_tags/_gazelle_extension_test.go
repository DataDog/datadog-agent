// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package go_build_tags

import (
	"go/build/constraint"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseConstraint(t *testing.T, expr string) constraint.Expr {
	t.Helper()
	e, err := constraint.Parse("//go:build " + expr)
	require.NoError(t, err)
	return e
}

func TestIsSystemTag(t *testing.T) {
	for _, tag := range []string{
		// GOOS
		"linux", "darwin", "windows", "freebsd", "android", "ios",
		// GOARCH
		"amd64", "arm64", "386", "arm", "wasm",
		// Go version
		"go1.17", "go1.21", "go1.24",
		// special compiler/mode tags
		"cgo", "gc", "gccgo", "ignore",
		// Go 1.19 pseudo-constraint covering all non-Windows GOOS values
		"unix",
	} {
		assert.True(t, isSystemTag(tag), "expected system tag: %s", tag)
	}

	for _, tag := range []string{
		"ec2", "test", "kubelet", "docker", "linux_bpf", "serverless", "npm", "trivy",
		// must not be confused with system tags
		"go2", "go", "cgoish", "linuxbox",
	} {
		assert.False(t, isSystemTag(tag), "expected user tag: %s", tag)
	}
}

func TestIsLinuxOnlyTag(t *testing.T) {
	for _, tag := range []string{"crio", "jetson", "linux_bpf", "netcgo", "nvml", "pcap", "podman", "systemd", "trivy"} {
		assert.True(t, isLinuxOnlyTag(tag), "expected linux-only tag: %s", tag)
	}
	for _, tag := range []string{"ec2", "kubelet", "docker", "test", "npm"} {
		assert.False(t, isLinuxOnlyTag(tag), "expected non-linux-only tag: %s", tag)
	}
}

func TestIsExcludedTag(t *testing.T) {
	assert.True(t, isExcludedTag("npm"), "npm must be excluded: requires Windows npm kernel driver")
	assert.False(t, isExcludedTag("ec2"), "ec2 must not be excluded")
	assert.False(t, isExcludedTag("trivy"), "trivy must not be excluded")
}

func TestPositiveUserTags(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{
			"single user tag",
			"ec2",
			[]string{"ec2"},
		},
		{
			"single system tag is filtered",
			"linux",
			nil,
		},
		{
			"AND: both user tags",
			"test && ec2",
			[]string{"test", "ec2"},
		},
		{
			"AND: system + user",
			"linux && ec2",
			[]string{"ec2"},
		},
		{
			// Only the left branch is needed to satisfy the OR; docker is not added.
			"OR: left branch wins",
			"kubelet || docker",
			[]string{"kubelet"},
		},
		{
			// Left branch is a system tag (filtered to nothing), so fall through to right.
			"OR: left is system tag, right is user tag",
			"linux || ec2",
			[]string{"ec2"},
		},
		{
			"NOT: skipped entirely",
			"!ec2",
			nil,
		},
		{
			"AND with NOT: NOT branch skipped",
			"test && !ec2",
			[]string{"test"},
		},
		{
			"AND+OR: OR already satisfied by prior AND branch",
			"ec2 && (ec2 || kubelet)",
			[]string{"ec2"},
		},
		{
			// Left branch of the OR (docker) is taken; containerd is not added.
			"AND+OR: OR not yet satisfied, left branch taken",
			"trivy && (docker || containerd)",
			[]string{"trivy", "docker"},
		},
		{
			// npm is excluded (requires Windows npm kernel driver); windows is a system tag.
			"excluded tag is filtered",
			"windows && npm",
			nil,
		},
		{
			// Left branch adds linux_bpf (linux is system tag, filtered); right branch not explored.
			"cross-platform OR: left branch has user tag",
			"(linux && linux_bpf) || (windows && npm)",
			[]string{"linux_bpf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := positiveUserTags(mustParseConstraint(t, tt.expr))
			assert.Equal(t, tt.want, got)
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestTagsFromFile(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			"simple user tag",
			"//go:build ec2\n\npackage foo\n",
			[]string{"ec2"},
		},
		{
			"system tag only",
			"//go:build linux\n\npackage foo\n",
			nil,
		},
		{
			"AND expression",
			"//go:build test && ec2\n\npackage foo\n",
			[]string{"test", "ec2"},
		},
		{
			// Left branch of the OR is taken; docker is not added.
			"OR expression",
			"//go:build kubelet || docker\n\npackage foo\n",
			[]string{"kubelet"},
		},
		{
			"no build constraint",
			"package foo\n",
			nil,
		},
		{
			"build constraint after package declaration",
			"package foo\n//go:build ec2\n",
			nil,
		},
		{
			"blank line before constraint",
			"\n//go:build ec2\n\npackage foo\n",
			[]string{"ec2"},
		},
		{
			"constraint preceded by another comment",
			"// Copyright blah blah\n//go:build ec2\n\npackage foo\n",
			[]string{"ec2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, dir, tt.name+".go", tt.content)
			got := tagsFromFile(path)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("missing file", func(t *testing.T) {
		got := tagsFromFile(filepath.Join(dir, "does_not_exist.go"))
		assert.Nil(t, got)
	})
}

func TestRequiredSourceTags(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "a.go", "//go:build ec2\n\npackage foo\n")
	writeFile(t, dir, "b.go", "//go:build kubelet || docker\n\npackage foo\n")
	writeFile(t, dir, "c.go", "//go:build ec2 && linux\n\npackage foo\n")
	writeFile(t, dir, "d.go", "package foo\n")
	writeFile(t, dir, "e.go", "//go:build !serverless\n\npackage foo\n")

	got := requiredSourceTags([]string{"a.go", "b.go", "c.go", "d.go", "e.go"}, dir)

	// ec2 from a.go; kubelet from b.go (left branch of OR, docker not added);
	// ec2 already seen from c.go (deduped); linux filtered (system tag);
	// !serverless from e.go skipped (NOT)
	assert.Equal(t, []string{"ec2", "kubelet"}, got)
}
