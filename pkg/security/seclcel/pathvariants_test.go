// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// symlinkEvent is an exec of a file reached through two symlinks, which is what SECL's
// resolver records when the binary it ran is one: the field holds the real path and the
// symlink pathnames hold the names it was invoked by.
func symlinkEvent() *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/dash"
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "dash"
	event.BaseEvent.ProcessContext.SymlinkPathnameStr[0] = "/bin/sh"
	event.BaseEvent.ProcessContext.SymlinkPathnameStr[1] = "/usr/bin/sh"
	event.BaseEvent.ProcessContext.SymlinkBasenameStr = "sh"
	return event
}

// overlayEvent is a file opened through an overlay mount, where the resolved path carries
// the mount point and the path a rule is written with does not.
func overlayEvent() *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.Open.File.PathnameStr = "/var/lib/docker/overlay2/abc/merged/etc/passwd"
	event.Open.File.BasenameStr = "passwd"
	event.Open.File.Filesystem = "overlay"
	event.Open.File.MountPath = "/var/lib/docker/overlay2/abc/merged"
	return event
}

// TestPathVariantsAgreeWithSECL is the differential over the paths a file field can be
// compared against.
//
// It is also what keeps the closures in pathvariants_unix.go in step with the model's own
// operator overrides, which are unexported and so had to be mirrored rather than called: if
// the model changes which paths it reaches for, these expressions stop agreeing.
func TestPathVariantsAgreeWithSECL(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	for _, tt := range []struct {
		name  string
		event *model.Event
		exprs []string
	}{
		{"symlink", symlinkEvent(), []string{
			// the field's own value
			`exec.file.path == "/usr/bin/dash"`,
			// and the names it was reached by, which only the variants see
			`exec.file.path == "/bin/sh"`,
			`exec.file.path == "/usr/bin/sh"`,
			`exec.file.path == "/bin/zsh"`,
			`exec.file.name == "sh"`,
			`exec.file.name == "dash"`,
			`exec.file.name == "zsh"`,

			// negated, where SECL negates the whole disjunction rather than each term
			`exec.file.path != "/bin/sh"`,
			`exec.file.path != "/usr/bin/dash"`,
			`exec.file.path != "/bin/zsh"`,
			`exec.file.name != "sh"`,

			// globs and patterns, which have to reach the variants too
			`exec.file.path =~ "/bin/*"`,
			`exec.file.path =~ "/opt/*"`,
			`exec.file.path !~ "/bin/*"`,
			`exec.file.name =~ "s*"`,

			// membership, over a plain list and over one holding patterns
			`exec.file.path in [ "/bin/sh", "/bin/zsh" ]`,
			`exec.file.path in [ "/bin/zsh" ]`,
			`exec.file.path in [ ~"/bin/*", ~"/opt/**" ]`,
			`exec.file.path not in [ ~"/bin/*", ~"/opt/**" ]`,
			`exec.file.name in [ "sh", "zsh" ]`,

			// the length is of the field itself, which carries no override
			`exec.file.path.length == 13`,

			// and the same field on the process, which the override also names
			`process.file.path == "/bin/sh"`,
		}},

		{"overlay", overlayEvent(), []string{
			`open.file.path == "/etc/passwd"`,
			`open.file.path == "/var/lib/docker/overlay2/abc/merged/etc/passwd"`,
			`open.file.path == "/etc/shadow"`,
			`open.file.path != "/etc/passwd"`,
			`open.file.path =~ "/etc/*"`,
			`open.file.path in [ ~"/etc/*" ]`,
			`open.file.path not in [ ~"/etc/*" ]`,
			// the basename has no overlay variant of its own
			`open.file.name == "passwd"`,
		}},

		{"neither", patternEvent(), []string{
			// nothing is a symlink and nothing is on an overlay mount, so the variants
			// are the field alone — the common case, and the one that must not change
			`process.file.path == "/usr/bin/bash"`,
			`process.file.path != "/usr/bin/bash"`,
			`process.file.path =~ "/usr/bin/*"`,
			`process.file.name == "bash"`,
			`process.file.path in [ "/usr/bin/bash" ]`,
		}},
	} {
		for _, expr := range tt.exprs {
			t.Run(tt.name+": "+expr, func(t *testing.T) {
				assert.Equal(t, evalSECLEngine(t, tt.event, expr), evalSECL(t, env, tt.event, expr),
					"the two engines disagree")
			})
		}
	}
}

// TestPathVariantsAreOnlyForTheFieldsThatHaveThem pins which fields carry variants, since
// giving one to a field SECL does not would make CEL the wider engine — the failure this
// step exists to remove, in the other direction.
func TestPathVariantsAreOnlyForTheFieldsThatHaveThem(t *testing.T) {
	types := ModelFieldTypes{}

	for _, field := range []string{
		// every file path has the overlay variant
		"exec.file.path", "process.file.path", "open.file.path", "unlink.file.path",
		"rename.file.destination.path", "process.parent.file.path",
		// and these two the symlink basename as well
		"exec.file.name", "process.file.name",
	} {
		assert.True(t, types.PathVariants(field), field)
	}

	for _, field := range []string{
		// a basename that is not a process's
		"open.file.name", "unlink.file.name",
		// a path of an iterated element, which the model refuses a file field for —
		// SECL's own override reads nothing there either
		"process.ancestors.file.path",
		// and anything that is not a path
		"process.comm", "open.flags", "process.uid",
	} {
		assert.False(t, types.PathVariants(field), field)
	}
}
