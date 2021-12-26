// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestGlobContains(t *testing.T) {
	if glob, _ := NewGlob("/var/log/*"); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log/httpd"); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log"); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd"); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd"); glob.Contains("/var/log/nginx") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/**"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/ng*"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/**"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}
}

func TestGlobMatches(t *testing.T) {
	if glob, _ := NewGlob("/var/log/*"); !glob.Matches("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**"); !glob.Matches("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	// FIX(safchain) once ** addressed
	/*if glob, _ := NewGlob("/var/*"); glob.Matches("/var/log/nginx") {
		t.Error("shouldn't contain the filename")
	}*/
}
