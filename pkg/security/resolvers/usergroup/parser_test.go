// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usergroup holds usergroup related files
package usergroup

import (
	"embed"
	"testing"
)

//go:embed passwd.sample
//go:embed group.sample
var testFS embed.FS

func TestPasswdParsing(t *testing.T) {
	users, err := parsePasswd(testFS, "passwd.sample")
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
	groups, err := parseGroup(testFS, "group.sample")
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
