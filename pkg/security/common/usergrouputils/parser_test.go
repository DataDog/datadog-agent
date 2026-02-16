// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usergroup holds usergroup related files
package usergroup

import (
	"embed"
	"strings"
	"testing"
)

//go:embed passwd.sample
//go:embed group.sample
var testFS embed.FS

func TestPasswdParsing(t *testing.T) {
	users, err := ParsePasswd(testFS, "passwd.sample")
	if err != nil {
		t.Error(err)
	}

	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	syslogUsername, found := users[104]
	if !found {
		t.Errorf("expected to find user with uid 104")
	}

	if syslogUsername != "syslog" {
		t.Errorf("expected user withuid 104 to be syslog")
	}
}

func TestGroupParsing(t *testing.T) {
	groups, err := ParseGroup(testFS, "group.sample")
	if err != nil {
		t.Error(err)
	}

	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}

	postdropGroupname, found := groups[147]
	if !found {
		t.Errorf("expected to find group with gid 147")
	}

	if postdropGroupname != "postdrop" {
		t.Errorf("expected group with gid 147 to be postdrop")
	}
}

func TestParsePasswdFileEdgeCases(t *testing.T) {
	input := strings.Join([]string{
		"# comment line",
		"",
		"root:x:0:0:root:/root:/bin/bash",
		"malformed-no-colons",
		"+nisuser:x:999:999:::",
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
		"baduid:x:notanumber:1000:::/bin/sh",
	}, "\n")

	users, err := ParsePasswdFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// root and nobody should be parsed
	if users[0] != "root" {
		t.Errorf("expected uid 0 = root, got %q", users[0])
	}
	if users[65534] != "nobody" {
		t.Errorf("expected uid 65534 = nobody, got %q", users[65534])
	}

	// malformed lines, +nis prefixed, and bad uids should be skipped
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d: %v", len(users), users)
	}
}

func TestParsePasswdFileEmpty(t *testing.T) {
	users, err := ParsePasswdFile(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestParseGroupFileEdgeCases(t *testing.T) {
	input := strings.Join([]string{
		"# group comment",
		"root:x:0:root",
		"malformed",
		"+nisgroup:x:999:",
		"nogroup:x:65534:",
		"badgid:x:notanumber:user",
	}, "\n")

	groups, err := ParseGroupFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if groups[0] != "root" {
		t.Errorf("expected gid 0 = root, got %q", groups[0])
	}
	if groups[65534] != "nogroup" {
		t.Errorf("expected gid 65534 = nogroup, got %q", groups[65534])
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d: %v", len(groups), groups)
	}
}
