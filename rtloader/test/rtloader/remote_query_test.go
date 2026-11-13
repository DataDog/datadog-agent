// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testrtloader

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

// RUN-01 / CHECK-03 bridge conformance, not integration behavior: prove UTF-8
// strings, callable discovery, and absence/non-string/exception failure handling.
func TestRemoteQueryBridge(t *testing.T) {
	for _, tc := range []struct {
		name, python, request, want string
		failure                     bool
	}{
		{"resolve", `reply = '{"status":"MATCHED"}'`, `{"operation":"resolve","target":{"database_instance":"orders"}}`, `{"status":"MATCHED"}`, false},
		{"execute", `reply = '{"status":"SUCCEEDED"}'`, `{"operation":"execute","query":"select 1"}`, `{"status":"SUCCEEDED"}`, false},
		{"unicode", `reply = '{"status":"FAILED","error":{"code":"query_failed","message":"caf\u00e9"}}'`, `{}`, `{"status":"FAILED","error":{"code":"query_failed","message":"café"}}`, false},
		{"non-string", `reply = None`, `{}`, "", true},
		{"exception", `fail = True`, `{}`, "", true},
		{"embedded NUL", `reply = 'ok\x00hidden'`, `{}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.ResetMemoryStats()
			code := `import remote_query_check as rq
rq.fail = False
rq.blocking = False
rq.received = []
` + "rq." + tc.python
			if _, err := runString(code); err != nil {
				t.Fatal(err)
			}
			c, err := newRemoteQueryCheck("remote_query_check")
			if err != nil {
				t.Fatal(err)
			}
			if !c.supported() {
				t.Fatal("callable remote_query not discovered")
			}
			out, err := c.query(tc.request)
			if tc.failure {
				if err == nil {
					t.Fatal("expected bridge failure")
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("Python exception data escaped")
				}
			} else if err != nil || out != tc.want {
				t.Fatalf("outcome = %q, error = %v", out, err)
			}
			c.close()
			if !tc.failure {
				out, err = runString(fmt.Sprintf("with open(r'%s', 'w') as f:\n    f.write(rq.received[-1])", tmpfile.Name()))
				if err != nil || out != tc.request {
					t.Fatalf("request changed: %q, %v", out, err)
				}
			}
			helpers.AssertMemoryUsage(t)
		})
	}
	t.Run("no method", func(t *testing.T) {
		c, err := newRemoteQueryCheck("fake_check")
		if err != nil {
			t.Fatal(err)
		}
		defer c.close()
		if c.supported() {
			t.Fatal("check without remote_query was considered supported")
		}
		if _, err = c.query(`{}`); err == nil {
			t.Fatal("missing method did not fail")
		}
	})
	t.Run("noncallable", func(t *testing.T) {
		_, err := runString(`import remote_query_check as rq
saved_method = rq.RemoteQueryCheck.remote_query
rq.RemoteQueryCheck.remote_query = None`)
		if err != nil {
			t.Fatal(err)
		}
		defer runString(`rq.RemoteQueryCheck.remote_query = saved_method`)
		c, err := newRemoteQueryCheck("remote_query_check")
		if err != nil {
			t.Fatal(err)
		}
		defer c.close()
		if c.supported() {
			t.Fatal("non-callable remote_query was considered supported")
		}
	})
}

// RUN-04 bridge conformance: cancel_remote_query can enter Python from another
// Go/OS thread while remote_query is blocked in Python I/O-like waiting.
func TestRemoteQueryCancellationBridge(t *testing.T) {
	_, err := runString(`import remote_query_check as rq
rq.fail = False
rq.blocking = True
rq.reply = '{"status":"FAILED","error":{"code":"cancelled","message":"cancelled"}}'
rq.entered.clear()
rq.released.clear()
rq.cancelled = []`)
	if err != nil {
		t.Fatal(err)
	}
	c, err := newRemoteQueryCheck("remote_query_check")
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	done := make(chan error, 1)
	go func() { _, err := c.query(`{"operation":"execute"}`); done <- err }()
	out, err := runString(fmt.Sprintf("with open(r'%s', 'w') as f:\n    f.write(str(rq.entered.wait(3)))", tmpfile.Name()))
	if err != nil || out != "True" {
		t.Fatalf("call did not enter Python: %q, %v", out, err)
	}
	if err := c.cancel("crawl-123"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote_query held GIL or was not cancelled")
	}
	out, err = runString(fmt.Sprintf("with open(r'%s', 'w') as f:\n    f.write(rq.cancelled[-1])\nrq.blocking = False", tmpfile.Name()))
	if err != nil || out != "crawl-123" {
		t.Fatalf("crawl ID changed: %q, %v", out, err)
	}
}
