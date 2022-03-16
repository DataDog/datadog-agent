// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"os"
	"text/template"

	"github.com/tinylib/msgp/msgp"
)

var profileTmpl = `---
name: {{ .Name }}
selector:
  - {{ .Selector }}

rules:{{ range .Rules }}
  - id: {{ .ID }}
    expression: {{ .Expression }}
{{ end }}
`

// GenerateProfile creates a profile from the input activity dump
func GenerateProfile(inputFile string) (string, error) {
	// open and parse activity dump file
	f, err := os.Open(inputFile)
	if err != nil {
		return "", fmt.Errorf("couldn't open activity dump file: %w", err)
	}

	var dump ActivityDump
	msgpReader := msgp.NewReader(f)
	err = dump.DecodeMsg(msgpReader)
	if err != nil {
		return "", fmt.Errorf("couldn't parse activity dump file: %w", err)
	}

	// create profile output file
	var profile *os.File
	profile, err = os.CreateTemp("/tmp", "profile-")
	if err != nil {
		return "", fmt.Errorf("couldn't create profile file: %w", err)
	}

	if err = os.Chmod(profile.Name(), 0400); err != nil {
		return "", fmt.Errorf("couldn't change the mode of the profile file: %w", err)
	}

	t := template.Must(template.New("tmpl").Parse(profileTmpl))
	err = t.Execute(profile, dump.GenerateProfileData())
	if err != nil {
		return "", fmt.Errorf("couldn't generate profile: %w", err)
	}

	return profile.Name(), nil
}
