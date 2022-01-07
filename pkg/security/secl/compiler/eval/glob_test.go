// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestGlobPattern(t *testing.T) {
	if _, err := NewGlob("**/conf.d"); err == nil {
		t.Error("should return an error")
	}

	if _, err := NewGlob("/etc/conf.d/**"); err != nil {
		t.Error("shouldn't return an error")
	}

	if _, err := NewGlob("/etc/**/*.conf"); err == nil {
		t.Error("should return an error")
	}

	if _, err := NewGlob("/etc/**.conf"); err == nil {
		t.Error("should return an error")
	}
}

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

	if glob, _ := NewGlob("*/http*"); glob.Contains("/var/log/httpd") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("*/*/http*"); !glob.Contains("/var/log/httpd") {
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

	if glob, _ := NewGlob("/var/*o*/nginx"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/**"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/ab*"); !glob.Contains("/etc/conf.d/") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*"); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log"); !glob.Contains("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/*"); glob.Contains("/etc/sys.d/nginx.conf") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/log"); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}
}

func TestGlobMatches(t *testing.T) {
	if glob, _ := NewGlob("/var/log/*"); !glob.Matches("/var/log/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/**"); !glob.Matches("/var/log/nginx") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/*"); glob.Matches("/var/log/nginx") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/log"); !glob.Matches("/var/log") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/run"); glob.Matches("/var/log") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run"); glob.Matches("/var/run/httpd") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run/*"); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("ab*"); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*b*"); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*d*"); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("*/*/httpd"); !glob.Matches("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/*/http*"); !glob.Matches("/var/log/httpd") {
		t.Error("should contain the filename")
	}
}
