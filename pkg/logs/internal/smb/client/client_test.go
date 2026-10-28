// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPath(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", ""},
		{"/", ""},
		{".", ""},
		{"app.log", "app.log"},
		{"/app/x.log", "app/x.log"},
		{"app//x.log/", "app/x.log"},
		{"./app/./x.log", "app/x.log"},
		{`app\sub\x.log`, "app/sub/x.log"},
		{`\\app\x.log`, "app/x.log"},
	} {
		got, err := CleanPath(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}

	for _, in := range []string{"..", "../x", "app/../../x", `app\..\x`, "a\x00b"} {
		_, err := CleanPath(in)
		assert.ErrorIs(t, err, os.ErrInvalid, in)
	}
}

func TestConfigNeverPrintsPassword(t *testing.T) {
	const password = "hunter2-Sup3rS3cret"
	cfg := Config{Host: "myacct.file.core.windows.net", Share: "logs", Username: "myacct", Password: password, DialTimeout: time.Second}

	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		out := fmt.Sprintf(verb, cfg)
		assert.NotContains(t, out, password, verb)
		assert.Contains(t, out, "myacct.file.core.windows.net", verb)
		out = fmt.Sprintf(verb, &cfg)
		assert.NotContains(t, out, password, verb)
	}
	assert.Contains(t, cfg.String(), "Password:********")

	cfg.Password = ""
	assert.Contains(t, cfg.String(), "Password: ", "an empty password prints as empty, not as a mask")
}

func TestConfigDefaultsAndValidation(t *testing.T) {
	cfg := Config{Host: "h", Share: "s"}.withDefaults()
	assert.Equal(t, 445, cfg.Port)
	assert.Equal(t, defaultDialTimeout, cfg.DialTimeout)
	assert.Equal(t, defaultOpTimeout, cfg.OpTimeout)
	assert.NoError(t, cfg.validate())
	assert.Equal(t, "smb://h/s", cfg.target())
	assert.Equal(t, "smb://h:1445/s", Config{Host: "h", Share: "s", Port: 1445}.target())

	for _, bad := range []Config{
		{Share: "s"},
		{Host: "h"},
		{Host: "h", Share: "a/b"},
		{Host: "h", Share: `a\b`},
		{Host: "h", Share: "s", Port: 70000},
		{Host: "h", Share: "s", Port: -1},
	} {
		assert.Error(t, bad.validate(), "%+v", bad)
	}
}

func TestNormalizeFileID(t *testing.T) {
	assert.Equal(t, uint64(0), normalizeFileID(0))
	assert.Equal(t, uint64(0), normalizeFileID(^uint64(0)))
	assert.Equal(t, uint64(42), normalizeFileID(42))
}

func TestIdentity(t *testing.T) {
	created := time.Date(2026, 10, 6, 12, 0, 0, 123456700, time.UTC)
	entry := Entry{Name: "app.log", FileID: 1081516, CreationTime: created}
	file := entry.Identity()
	assert.Equal(t, Identity{FileID: 1081516, Created: created.UnixNano()}, file)
	assert.Equal(t, file, ReadResult{FileID: 1081516, CreationTime: created.In(time.Local)}.Identity(), "the location does not matter")
	assert.Equal(t, "FileId 1081516 created 2026-10-06T12:00:00.1234567Z", file.String())
	assert.Equal(t, "FileId 7", Identity{FileID: 7}.String())

	// The FILETIME epoch, which servers report for an unknown creation time,
	// and the zero time are unknown creation times.
	filetimeEpoch := time.Unix(-11644473600, 0)
	assert.Equal(t, Identity{FileID: 7}, Entry{FileID: 7, CreationTime: filetimeEpoch}.Identity())
	assert.Equal(t, Identity{FileID: 7}, ReadResult{FileID: 7}.Identity())

	reused := Identity{FileID: file.FileID, Created: file.Created + 1}
	for _, tc := range []struct {
		a, b Identity
		want bool
	}{
		{file, file, true},
		{file, reused, false}, // a new file with the FileId of a deleted one
		{file, Identity{FileID: 1, Created: file.Created}, false}, // another FileId
		{file, Identity{FileID: file.FileID}, true},               // creation time unknown on one side
		{file, Identity{Created: file.Created}, true},             // FileId unknown on one side
		{file, Identity{Created: reused.Created}, false},
		{Identity{}, Identity{}, true},
	} {
		assert.Equal(t, tc.want, tc.a.Matches(tc.b), "%v / %v", tc.a, tc.b)
		assert.Equal(t, tc.want, tc.b.Matches(tc.a), "%v / %v", tc.b, tc.a)
	}
}
