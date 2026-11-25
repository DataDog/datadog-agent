// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usage

import (
	"slices"
	"testing"
)

func TestClean(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"usr/bin/bash", "/usr/bin/bash"},
		{"/usr/bin/bash", "/usr/bin/bash"},
		{"/./usr//lib/../bin/bash", "/usr/bin/bash"},
		{"app/node_modules/express/package.json", "/app/node_modules/express/package.json"},
	} {
		if got := Clean(tc.in); got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestAppendKeys checks the keys through which the run of a file credits a
// component: the anchors the index holds for Node.js packages, Ruby gems and
// Python bytecode, and the path itself for any other file.
func TestAppendKeys(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/usr/bin/bash", []string{"/usr/bin/bash"}},
		{"/app/node_modules/express/lib/router/index.js", []string{"/app/node_modules/express/package.json"}},
		{"/app/node_modules/@babel/core/lib/index.js", []string{"/app/node_modules/@babel/core/package.json"}},
		{"/app/node_modules/express/node_modules/debug/src/index.js", []string{"/app/node_modules/express/node_modules/debug/package.json"}},
		{"/app/node_modules/.pnpm/express@4.18.2/node_modules/express/index.js", []string{"/app/node_modules/.pnpm/express@4.18.2/node_modules/express/package.json"}},
		{"/app/node_modules/bcrypt/build/Release/bcrypt_lib.node", []string{"/app/node_modules/bcrypt/package.json"}},
		// A file of node_modules outside a package is a file of its own.
		{"/app/node_modules/.bin/tsc", []string{"/app/node_modules/.bin/tsc"}},
		{"/app/node_modules/@babel", []string{"/app/node_modules/@babel"}},
		{"/usr/local/bundle/gems/rake-13.0.6/lib/rake.rb", []string{"/usr/local/bundle/specifications/rake-13.0.6.gemspec"}},
		{"/usr/lib/ruby/gems/3.2.0/gems/rake-13.0.6/exe/rake", []string{"/usr/lib/ruby/gems/3.2.0/specifications/rake-13.0.6.gemspec"}},
		{"/usr/lib/ruby/gems/3.2.0/extensions/x86_64-linux/3.2.0/nokogiri-1.15.4/nokogiri/nokogiri.so", []string{"/usr/lib/ruby/gems/3.2.0/specifications/nokogiri-1.15.4.gemspec"}},
		{"/usr/local/bundle/gems/foo-1.0/extensions/a/b/c/d.rb", []string{"/usr/local/bundle/specifications/foo-1.0.gemspec"}},
		{"/usr/local/bundle/gems/foo-1.0/vendor/gems/bar/x.rb", []string{"/usr/local/bundle/specifications/foo-1.0.gemspec"}},
		// A gems directory that holds no gem is a directory of its own.
		{"/srv/gems/README", []string{"/srv/gems/README"}},
		{"/usr/lib/python3/dist-packages/requests/__pycache__/api.cpython-312.pyc", []string{
			"/usr/lib/python3/dist-packages/requests/__pycache__/api.cpython-312.pyc",
			"/usr/lib/python3/dist-packages/requests/api.py",
		}},
	} {
		var want []uint64
		for _, p := range tc.want {
			want = append(want, Key(p))
		}
		if got := AppendKeys(nil, tc.path); !slices.Equal(got, want) {
			t.Errorf("AppendKeys(%q) = %x, want the keys of %q", tc.path, got, tc.want)
		}
	}
}

func TestSourceKey(t *testing.T) {
	key, ok := SourceKey("/usr/lib/python3/dist-packages/requests/__pycache__/api.cpython-312.pyc")
	if !ok || key != Key("/usr/lib/python3/dist-packages/requests/api.py") {
		t.Errorf("SourceKey of bytecode = %x, %v, want the key of its source", key, ok)
	}
	for _, p := range []string{"/usr/lib/python3/dist-packages/requests/api.py", "/opt/app/api.pyc"} {
		if _, ok := SourceKey(p); ok {
			t.Errorf("SourceKey(%q) reports bytecode", p)
		}
	}
}

func TestPathAlias(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/bin/bash", "/usr/bin/bash"},
		{"/usr/bin/bash", "/bin/bash"},
		{"/sbin/ip", "/usr/sbin/ip"},
		{"/lib64/libc.so.6", "/usr/lib64/libc.so.6"},
		{"/usr/lib/x86_64-linux-gnu/libc.so.6", "/lib/x86_64-linux-gnu/libc.so.6"},
		{"/usr/share/zoneinfo/UTC", ""},
		{"/opt/app/bin/app", ""},
		{"/usr/libexec/sudo/sudoers.so", ""},
		{"/libexec/sudo/sudoers.so", ""},
		{"/binaries/app", ""},
		{"/usr/libx32/libc.so.6", "/libx32/libc.so.6"},
	} {
		if got := pathAlias(tc.in); got != tc.want {
			t.Errorf("pathAlias(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
