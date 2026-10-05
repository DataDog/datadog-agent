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
