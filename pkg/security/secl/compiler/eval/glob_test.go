// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"testing"
)

func TestGlobPattern(t *testing.T) {
	if _, err := NewGlob("/etc/**/**/conf.d", false, false); err == nil {
		t.Error("should return an error")
	}
}

func TestGlobIsPrefix(t *testing.T) {
	if glob, _ := NewGlob("/", false, false); !glob.IsPrefix("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false, false); !glob.IsPrefix("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log/httpd", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/log", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("*/http*", false, false); glob.IsPrefix("/var/log/httpd") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("*/*/http*", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*/httpd", false, false); glob.IsPrefix("/var/log/nginx") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/**", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/ng*", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*o*/nginx", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log/**", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/ab*", false, false); !glob.IsPrefix("/etc/conf.d/") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/*", false, false); !glob.IsPrefix("/var/log/nginx") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/log", false, false); !glob.IsPrefix("/var/log") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/etc/conf.d/*", false, false); glob.IsPrefix("/etc/sys.d/nginx.conf") {
		t.Error("shouldn't contain the filename")
	}

	if glob, _ := NewGlob("/var/log", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("/var/**/httpd", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}

	if glob, _ := NewGlob("**/httpd", false, false); !glob.IsPrefix("/var/log/httpd") {
		t.Error("should contain the filename")
	}
}

func TestGlobMatches(t *testing.T) {
	if glob, _ := NewGlob("/tmp/test/test789", false, false); !glob.Matches("/tmp/test/test789") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*/abc/*", false, false); !glob.Matches("/1/abc/2") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/tmp/test/test789*", false, false); !glob.Matches("/tmp/test/test7890") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/tmp/test/test789*", false, false); glob.Matches("/tmp/test") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/tmp/test/*st*", false, false); glob.Matches("/tmp/test") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/tmp/test/*st*", false, false); !glob.Matches("/tmp/test/ast") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/tmp/*/test789", false, false); !glob.Matches("/tmp/test/test789") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/tmp/*/test789", false, false); glob.Matches("/tmp/test/test") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/tmp/**", false, false); !glob.Matches("/tmp/test/ast") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/tmp/*", false, false); glob.Matches("/tmp/test/ast") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("*", false, false); glob.Matches("/tmp/test/ast") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("**", false, false); !glob.Matches("/tmp/test/ast") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/log/*", false, false); !glob.Matches("/var/log/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/**", false, false); !glob.Matches("/var/log/nginx") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/*", false, false); glob.Matches("/var/log/nginx") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/log", false, false); !glob.Matches("/var/log") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/var/run", false, false); glob.Matches("/var/log") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run", false, false); glob.Matches("/var/run/httpd") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/var/run/*", false, false); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("ab*", false, false); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*b*", false, false); !glob.Matches("abc") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*d*", false, false); glob.Matches("abc") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("*/*/httpd", false, false); !glob.Matches("/var/log/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*/*/http*", false, false); !glob.Matches("/var/log/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("httpd", false, false); !glob.Matches("httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("mysqld", false, false); glob.Matches("httpd") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/", false, false); glob.Matches("/httpd") {
		t.Error("shouldn't match the filename")
	}

	if glob, _ := NewGlob("/*", false, false); !glob.Matches("/httpd") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/sys/fs/cgroup/*", false, false); !glob.Matches("/sys/fs/cgroup/") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/sys/fs/cgroup/**", false, false); !glob.Matches("/sys/fs/cgroup/") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/sys/fs*", false, false); !glob.Matches("/sys/fsa") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("*/*/http*/*b*/test/**", false, false); !glob.Matches("/var/log/httpd/abc/test/123") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("**", false, false); !glob.Matches("/var/log/httpd/abc/test/123") {
		t.Error("should match the filename")
	}

	// with prefix
	if glob, _ := NewGlob("**/cgroup", false, false); !glob.Matches("/sys/fs/cgroup") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("**/fs/cgroup", false, false); !glob.Matches("/sys/fs/cgroup") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("**/f*/cgr*", false, false); !glob.Matches("/sys/fs/cgroup") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/sys/**/cgr*", false, false); !glob.Matches("/sys/fs/test/cgroup") {
		t.Error("should match the filename")
	}

	if glob, _ := NewGlob("/sys/**/abs/cgr*", false, false); glob.Matches("/sys/fs/test/cgroup") {
		t.Error("shouldn't match the filename")
	}
}

func FuzzGlob(f *testing.F) {
	f.Add("**/cgroup", "/sys/fs/cgroup")
	f.Add("**/fs/cgroup", "/sys/fs/cgroup")
	f.Add("**/f*/cgr*", "/sys/fs/cgroup")
	f.Add("**/cgr*", "/sys/fs/cgroup")
	f.Add("**/cgr*", "/sys/fs/cgroup")
	f.Add("/sys/**/cgr*", "/sys/fs/cgroup")
	f.Add("/sys/**", "/sys/fs/cgroup")
	f.Add("/sys/*/cgr*", "/sys/fs/cgroup")
	f.Add("/sys/fs/cgroup/*", "/sys/fs/cgroup/")
	f.Add("/sys/fs/cgroup/**", "/sys/fs/cgroup/")
	f.Add("/sys/fs*", "/sys/fsa")
	f.Add("*/*/http*/*b*/test/**", "/var/log/httpd/abc/test/123")
	f.Add("**", "/var/log/httpd/abc/test/123")

	f.Fuzz(func(_ *testing.T, pattern string, filename string) {
		glob, err := NewGlob(pattern, false, false)
		if err != nil {
			return
		}
		glob.Matches(filename)
	})
}

func BenchmarkGlob(b *testing.B) {
	glob, err := NewGlob("*/*/http*/*b*/test/**", false, false)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()

	for b.Loop() {
		if matches := glob.Matches("/var/log/httpd/abc/test/123"); !matches {
			b.Fatalf("glob should match")
		}
	}
}
