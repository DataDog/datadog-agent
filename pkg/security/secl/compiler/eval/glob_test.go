// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"
)

func TestGlobPattern(t *testing.T) {
	if _, err := NewGlob("**/conf.d", false); err == nil {
		t.Error("should return an error")
	}

	if _, err := NewGlob("/etc/conf.d/**", false); err != nil {
		t.Error("shouldn't return an error")
	}

	if _, err := NewGlob("/etc/**/*.conf", false); err == nil {
		t.Error("should return an error")
	}

	if _, err := NewGlob("/etc/**.conf", false); err == nil {
		t.Error("should return an error")
	}
}

func TestGlobContains(t *testing.T) {
	if glob, _ := NewGlob("/", false); !glob.Contains("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false); !glob.Contains("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log/httpd", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/http*", false); glob.Contains("/var/log/httpd") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("*/*/http*", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd", false); glob.Contains("/var/log/nginx") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/**", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/ng*", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*o*/nginx", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/**", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/ab*", false); !glob.Contains("/etc/conf.d/") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*", false); !glob.Contains("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log", false); !glob.Contains("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/*", false); glob.Contains("/etc/sys.d/nginx.conf") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/log", false); !glob.Contains("/var/log/httpd") {
		t.Error("should contain the filename")
	}
}

func TestGlobMatches(t *testing.T) {
	if glob, _ := NewGlob("/tmp/test/test789", false); !glob.Matches("/tmp/test/test789") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/abc/*", false); !glob.Matches("/1/abc/2") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/tmp/test/test789*", false); !glob.Matches("/tmp/test/test7890") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/tmp/test/test789*", false); glob.Matches("/tmp/test") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/tmp/test/*st*", false); glob.Matches("/tmp/test") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/tmp/test/*st*", false); !glob.Matches("/tmp/test/ast") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/tmp/*/test789", false); !glob.Matches("/tmp/test/test789") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/tmp/*/test789", false); glob.Matches("/tmp/test/test") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/tmp/**", false); !glob.Matches("/tmp/test/ast") {
		t.Error("should the filename")
	}

	if glob, _ := NewGlob("/tmp/*", false); glob.Matches("/tmp/test/ast") {
		t.Error("shouldn't the filename")
	}

	if glob, _ := NewGlob("*", false); glob.Matches("/tmp/test/ast") {
		t.Error("shouldn't the filename")
	}

	if glob, _ := NewGlob("**", false); !glob.Matches("/tmp/test/ast") {
		t.Error("should the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false); !glob.Matches("/var/log/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/**", false); !glob.Matches("/var/log/nginx") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/*", false); glob.Matches("/var/log/nginx") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/log", false); !glob.Matches("/var/log") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/run", false); glob.Matches("/var/log") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run", false); glob.Matches("/var/run/httpd") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run/*", false); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("ab*", false); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*b*", false); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*d*", false); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("*/*/httpd", false); !glob.Matches("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/*/http*", false); !glob.Matches("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("httpd", false); !glob.Matches("httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("mysqld", false); glob.Matches("httpd") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/", false); glob.Matches("/httpd") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/*", false); !glob.Matches("/httpd") {
		t.Error("should contain the filename")
	}
}
