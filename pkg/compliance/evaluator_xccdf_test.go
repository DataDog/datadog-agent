// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// hangTimeout is the wait for a result in the cases where the fake oscap-io
// hangs. It exceeds the start time of the test binary.
const hangTimeout = 10 * time.Second

// TestMain runs the test binary as a fake oscap-io when the evaluator tests
// start it with GO_WANT_HELPER_OSCAP_IO set.
func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_OSCAP_IO") == "1" {
		fakeOSCAPIO()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeOSCAPIO speaks the oscap-io protocol on its standard input and output.
// It answers each request after the suffix of the rule id.
func fakeOSCAPIO() {
	codes := map[string]int{
		"pass":       xccdfPass,
		"fail":       xccdfFail,
		"error":      xccdfError,
		"unknown":    xccdfUnknown,
		"skip":       xccdfNotApplicable,
		"notchecked": xccdfNotChecked,
		"unscored":   xccdfInformational,
		"fixed":      xccdfFixed,
	}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		_, id, _ := strings.Cut(s.Text(), " ")
		switch suffix := id[strings.LastIndexByte(id, '_')+1:]; suffix {
		case "silent":
			fmt.Fprintf(os.Stderr, "error: xccdf_session_evaluate: %s\n", id)
		case "garbage":
			fmt.Printf("garbage\nunrequested_pass 1\n%s 42\n%s 1\n%s 2\n", id, id, id)
		case "stderr":
			fmt.Fprint(os.Stderr, strings.Repeat("W: oscap: probe warning\n", 5000))
			fmt.Printf("%s %d\n", id, xccdfPass)
		case "crash":
			os.Exit(1)
		case "abort":
			fmt.Printf("%s %d\n", id, xccdfPass)
			os.Exit(1)
		case "hang":
			// A select {} would end the process as a deadlock.
			time.Sleep(time.Hour)
		default:
			fmt.Printf("%s %d\n", id, codes[suffix])
		}
	}
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// fakeOSCAPIOCommand returns a command running the test binary as fakeOSCAPIO.
func fakeOSCAPIOCommand(ctx context.Context) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_OSCAP_IO=1")
	return cmd, nil
}

// evaluateFake evaluates the XCCDF rule ids with fakeOSCAPIO, each on behalf
// of a rule of the same id, and returns "rule result" for each event.
func evaluateFake(ctx context.Context, ids []string, interval, timeout time.Duration) ([]string, error) {
	reqs := make([]xccdfRequest, len(ids))
	for i, id := range ids {
		reqs[i] = xccdfRequest{profile: "profile", id: id, rule: &Rule{ID: id}}
	}
	o := &oscapIO{name: "test", newCmd: fakeOSCAPIOCommand, interval: interval, timeout: timeout}
	results, err := o.evaluate(ctx, reqs)
	var got []string
	for _, e := range newXCCDFEvents("host", &Benchmark{FrameworkID: "test"}, results) {
		got = append(got, e.RuleID+" "+string(e.Result))
	}
	return got, err
}

func TestOSCAPIOEvaluate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ids      []string
		interval time.Duration
		timeout  time.Duration
		want     []string
		wantErr  bool
	}{
		{
			name:    "codes",
			ids:     []string{"a_pass", "b_fail", "c_error", "d_unknown", "e_skip", "f_notchecked", "g_unscored", "h_fixed"},
			timeout: time.Minute,
			want:    []string{"a_pass passed", "b_fail failed", "c_error error", "d_unknown error", "e_skip skipped"},
		},
		{
			name:     "paced",
			ids:      []string{"a_pass", "b_fail", "c_pass"},
			interval: 10 * time.Millisecond,
			timeout:  time.Minute,
			want:     []string{"a_pass passed", "b_fail failed", "c_pass passed"},
		},
		{
			name:    "silent",
			ids:     []string{"a_silent", "b_pass", "c_silent", "d_silent"},
			timeout: time.Minute,
			want:    []string{"b_pass passed"},
		},
		{
			name:    "garbage",
			ids:     []string{"a_garbage", "b_fail"},
			timeout: time.Minute,
			want:    []string{"a_garbage passed", "b_fail failed"},
		},
		{
			name:    "stderr",
			ids:     []string{"a_stderr", "b_fail"},
			timeout: time.Minute,
			want:    []string{"a_stderr passed", "b_fail failed"},
		},
		{
			name:    "crash",
			ids:     []string{"a_pass", "b_crash", "c_fail"},
			timeout: time.Minute,
			want:    []string{"a_pass passed", "c_fail failed"},
		},
		{
			name:    "hang",
			ids:     []string{"a_pass", "b_hang", "c_fail"},
			timeout: hangTimeout,
			want:    []string{"a_pass passed", "c_fail failed"},
		},
		{
			name:    "hang first",
			ids:     []string{"a_hang", "b_pass"},
			timeout: hangTimeout,
			want:    []string{"b_pass passed"},
		},
		{
			name:    "abort after each answer",
			ids:     []string{"a_abort", "b_abort", "c_fail"},
			timeout: time.Minute,
			want:    []string{"a_abort passed", "b_abort passed", "c_fail failed"},
		},
		{
			name:    "no answer",
			ids:     []string{"a_crash", "b_crash", "c_pass"},
			timeout: time.Minute,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := evaluateFake(t.Context(), tt.ids, tt.interval, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluate(%q) error = %v, want error %t", tt.ids, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("evaluate(%q) = %q, want %q", tt.ids, got, tt.want)
			}
		})
	}
}

func TestOSCAPIOEvaluateDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), hangTimeout)
	defer cancel()
	ids := []string{"a_pass", "b_hang", "c_fail"}
	got, err := evaluateFake(ctx, ids, 0, time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("evaluate(%q) error = %v, want %v", ids, err, context.DeadlineExceeded)
	}
	if want := []string{"a_pass passed"}; !slices.Equal(got, want) {
		t.Errorf("evaluate(%q) = %q, want %q", ids, got, want)
	}
}

func TestXCCDFRequests(t *testing.T) {
	xccdf := func(name, rule string, rules ...string) []*InputSpec {
		return []*InputSpec{{XCCDF: &InputSpecXCCDF{Name: name, Profile: "profile", Rule: rule, Rules: rules}}}
	}
	benchmark := &Benchmark{
		dirname:     "compliance.d",
		FrameworkID: "test",
		Rules: []*Rule{
			{ID: "a", InputSpecs: xccdf("ds.xml", "a_pass")},
			{ID: "rego", InputSpecs: []*InputSpec{{File: &InputSpecFile{Path: "/etc/passwd"}}}},
			{ID: "b", InputSpecs: xccdf("ds.xml", "", "b_pass", "b_fail")},
		},
	}
	file, reqs, err := xccdfRequests(benchmark)
	if err != nil {
		t.Fatalf("xccdfRequests() error = %v", err)
	}
	if want := filepath.Join("compliance.d", "ds.xml"); file != want {
		t.Errorf("xccdfRequests() file = %q, want %q", file, want)
	}
	var got []string
	for _, r := range reqs {
		got = append(got, r.rule.ID+" "+r.profile+" "+r.id)
	}
	if want := []string{"a profile a_pass", "b profile b_pass", "b profile b_fail"}; !slices.Equal(got, want) {
		t.Errorf("xccdfRequests() = %q, want %q", got, want)
	}

	benchmark.Rules = append(benchmark.Rules, &Rule{ID: "c", InputSpecs: xccdf("other.xml", "c_pass")})
	if _, _, err := xccdfRequests(benchmark); err == nil {
		t.Errorf("xccdfRequests() with two datastreams: error = nil, want an error")
	}
}
