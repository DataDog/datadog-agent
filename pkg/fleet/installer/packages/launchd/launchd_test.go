// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runningPrintOutput is real `launchctl print system/com.apple.opendirectoryd` output, trimmed to
// the leading block. The parser must survive launchd's nested, undocumented dump, so the fixture
// keeps the nesting rather than a hand-written flat list of the keys under test.
const runningPrintOutput = `system/com.datadoghq.agent = {
	active count = 7
	path = /Library/LaunchDaemons/com.datadoghq.agent.plist
	type = LaunchDaemon
	state = running

	program = /opt/datadog-agent/bin/agent/agent
	default environment = {
		PATH => /usr/bin:/bin:/usr/sbin:/sbin
	}

	environment = {
		OSLogRateLimit => 64
		XPC_SERVICE_NAME => com.datadoghq.agent
	}

	domain = system
	minimum runtime = 10
	exit timeout = 5
	runs = 1
	pid = 395
	forks = 0
	execs = 1
	last exit code = (never exited)
}
`

// exitedPrintOutput is a job launchd has run and recorded an exit code for.
const exitedPrintOutput = `system/com.datadoghq.agent-exp = {
	active count = 0
	path = /Library/LaunchDaemons/com.datadoghq.agent-exp.plist
	type = LaunchDaemon
	state = not running

	program = /opt/datadog-agent/bin/agent/agent
	domain = system
	runs = 1
	last exit code = 2
}
`

// notLoadedOutput is real launchctl output for a label that is not in the domain. launchctl exits
// 113 for it, which the client must read as "not loaded" rather than as a failure.
const notLoadedOutput = `Bad request.
Could not find service "com.datadoghq.agent" in domain for system
`

type call struct {
	name string
	args []string
}

// recorder is a launchctl stand-in: it records the invocations and replays a queued result per
// call, so the argument construction and the output parsing are both testable without launchd.
type recorder struct {
	calls   []call
	outputs [][]byte
	errs    []error
}

func (r *recorder) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, call{name: name, args: args})
	var out []byte
	var err error
	if len(r.outputs) > 0 {
		out, r.outputs = r.outputs[0], r.outputs[1:]
	}
	if len(r.errs) > 0 {
		err, r.errs = r.errs[0], r.errs[1:]
	}
	return out, err
}

func newClient(rec *recorder) *Client {
	c := NewClient(System)
	c.Runner = rec.run
	return c
}

func TestParsePrint(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want JobStatus
	}{
		{
			name: "running job",
			out:  runningPrintOutput,
			want: JobStatus{Label: "com.datadoghq.agent", PID: 395, LastExitStatus: 0, Loaded: true},
		},
		{
			name: "job that exited non-zero",
			out:  exitedPrintOutput,
			want: JobStatus{Label: "com.datadoghq.agent", PID: 0, LastExitStatus: 2, Loaded: true},
		},
		{
			name: "loaded job launchd reports nothing else about",
			out:  "system/com.datadoghq.agent = {\n\tstate = not running\n}\n",
			want: JobStatus{Label: "com.datadoghq.agent", Loaded: true},
		},
		{
			// "pid" appears inside nested dictionaries as a substring of other keys
			// (e.g. "spawn type"); only a whole-line match may be taken.
			name: "pid-like keys are not mistaken for the pid",
			out:  "system/com.datadoghq.agent = {\n\tinherited pid = 1\n\tpid = 42\n}\n",
			want: JobStatus{Label: "com.datadoghq.agent", PID: 42, Loaded: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parsePrint("com.datadoghq.agent", tt.out))
		})
	}
}

func TestPrintNotLoadedIsNotAnError(t *testing.T) {
	rec := &recorder{outputs: [][]byte{[]byte(notLoadedOutput)}, errs: []error{errors.New("exit status 113")}}
	status, err := newClient(rec).Print(context.Background(), "com.datadoghq.agent")
	require.NoError(t, err)
	assert.Equal(t, JobStatus{Label: "com.datadoghq.agent"}, status)
	assert.False(t, status.Loaded)
}

func TestPrintRealFailureIsAnError(t *testing.T) {
	rec := &recorder{outputs: [][]byte{[]byte("Bad request.\nCould not talk to launchd\n")}, errs: []error{errors.New("exit status 1")}}
	_, err := newClient(rec).Print(context.Background(), "com.datadoghq.agent")
	assert.Error(t, err)
}

func TestLoaded(t *testing.T) {
	rec := &recorder{outputs: [][]byte{[]byte(runningPrintOutput)}}
	loaded, err := newClient(rec).Loaded(context.Background(), "com.datadoghq.agent")
	require.NoError(t, err)
	assert.True(t, loaded)

	rec = &recorder{outputs: [][]byte{[]byte(notLoadedOutput)}, errs: []error{errors.New("exit status 113")}}
	loaded, err = newClient(rec).Loaded(context.Background(), "com.datadoghq.agent")
	require.NoError(t, err)
	assert.False(t, loaded)
}

// TestOperationsAreIdempotentWhenJobIsAbsent covers the property the hooks rely on: they run on
// both install paths and may run again on a host already in the desired state.
func TestOperationsAreIdempotentWhenJobIsAbsent(t *testing.T) {
	t.Run("bootout of a job that is not loaded", func(t *testing.T) {
		for _, out := range []string{
			notLoadedOutput,
			"Boot-out failed: 3: No such process\n",
		} {
			// A second response is queued for the settle loop's print poll: the job was
			// already absent, so the very first check must confirm that and return.
			rec := &recorder{
				outputs: [][]byte{[]byte(out), []byte(notLoadedOutput)},
				errs:    []error{errors.New("exit status 3"), errors.New("exit status 113")},
			}
			assert.NoError(t, newClient(rec).Bootout(context.Background(), "com.datadoghq.agent"))
		}
	})

	t.Run("bootstrap of a job that is already loaded", func(t *testing.T) {
		for _, out := range []string{
			"Bootstrap failed: 37: Operation already in progress\n",
			"Bootstrap failed: 17: File exists\n",
			"Load failed: 5: Input/output error: service already loaded\n",
		} {
			rec := &recorder{outputs: [][]byte{[]byte(out)}, errs: []error{errors.New("exit status 5")}}
			job := Job{Label: "com.datadoghq.agent", Domain: System}
			assert.NoError(t, newClient(rec).Bootstrap(context.Background(), job))
		}
	})

	t.Run("removing a definition that is not there", func(t *testing.T) {
		job := Job{Label: "com.datadoghq.agent", PlistPath: filepath.Join(t.TempDir(), "absent.plist")}
		assert.NoError(t, job.Remove())
		assert.NoError(t, job.Remove())
	})
}

// TestBootoutRealFailureIsAnError guards the idempotency above from swallowing a genuine failure:
// a bootout refused for any reason other than the job being absent must surface.
func TestBootoutRealFailureIsAnError(t *testing.T) {
	rec := &recorder{outputs: [][]byte{[]byte("Boot-out failed: 1: Operation not permitted\n")}, errs: []error{errors.New("exit status 1")}}
	assert.Error(t, newClient(rec).Bootout(context.Background(), "com.datadoghq.agent"))
}

// TestBootoutNamesTheServiceTarget pins the launchctl invocation bootout makes, independent of the
// settle loop that follows it.
func TestBootoutNamesTheServiceTarget(t *testing.T) {
	rec := &recorder{outputs: [][]byte{nil, []byte(notLoadedOutput)}, errs: []error{nil, errors.New("exit status 113")}}
	require.NoError(t, newClient(rec).Bootout(context.Background(), "com.datadoghq.agent"))
	require.NotEmpty(t, rec.calls)
	assert.Equal(t, []string{"bootout", "system/com.datadoghq.agent"}, rec.calls[0].args)
}

// TestBootoutWaitsForTheJobToLeaveTheDomain guards the fix for the race documented in
// priv_notes/TO_BE_FIXED_MACOS_LAUNCHD_BOOTOUT_BOOTSTRAP_RACE.md: launchctl bootout can return
// before the label has actually left the domain, and a Bootstrap that follows immediately can hit
// a bare "Input/output error". Bootout must not return until a print poll confirms the label is
// gone.
func TestBootoutWaitsForTheJobToLeaveTheDomain(t *testing.T) {
	rec := &recorder{
		outputs: [][]byte{
			nil,                        // bootout
			[]byte(runningPrintOutput), // print: still loaded
			[]byte(runningPrintOutput), // print: still loaded
			[]byte(notLoadedOutput),    // print: finally gone
		},
		errs: []error{nil, nil, nil, errors.New("exit status 113")},
	}
	c := newClient(rec)
	c.BootoutSettlePollInterval = time.Millisecond
	require.NoError(t, c.Bootout(context.Background(), "com.datadoghq.agent"))

	var prints int
	for _, call := range rec.calls {
		if len(call.args) > 0 && call.args[0] == "print" {
			prints++
		}
	}
	assert.Equal(t, 3, prints, "Bootout must poll print until the label is actually gone")
}

// TestBootoutTimesOutIfTheJobNeverLeaves is the regression guard for the settle loop itself: it
// must fail loudly rather than hang forever or silently report success on a label that never
// actually left the domain.
func TestBootoutTimesOutIfTheJobNeverLeaves(t *testing.T) {
	c := NewClient(System)
	c.Runner = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(runningPrintOutput), nil
	}
	c.BootoutSettleTimeout = 10 * time.Millisecond
	c.BootoutSettlePollInterval = time.Millisecond
	assert.Error(t, c.Bootout(context.Background(), "com.datadoghq.agent"))
}

func TestCommandConstruction(t *testing.T) {
	ctx := context.Background()
	job := Job{Label: "com.datadoghq.agent", Domain: System}

	tests := []struct {
		name string
		do   func(c *Client) error
		want []string
	}{
		{
			name: "bootstrap names the domain and the definition path",
			do:   func(c *Client) error { return c.Bootstrap(ctx, job) },
			want: []string{"bootstrap", "system", "/Library/LaunchDaemons/com.datadoghq.agent.plist"},
		},
		{
			name: "kickstart without kill",
			do:   func(c *Client) error { return c.Kickstart(ctx, "com.datadoghq.agent", false) },
			want: []string{"kickstart", "system/com.datadoghq.agent"},
		},
		{
			name: "kickstart with kill",
			do:   func(c *Client) error { return c.Kickstart(ctx, "com.datadoghq.agent", true) },
			want: []string{"kickstart", "-k", "system/com.datadoghq.agent"},
		},
		{
			name: "enable",
			do:   func(c *Client) error { return c.Enable(ctx, "com.datadoghq.agent") },
			want: []string{"enable", "system/com.datadoghq.agent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			require.NoError(t, tt.do(newClient(rec)))
			require.Len(t, rec.calls, 1)
			assert.Equal(t, "/bin/launchctl", rec.calls[0].name)
			assert.Equal(t, tt.want, rec.calls[0].args)
		})
	}
}

func TestGUIDomainTargets(t *testing.T) {
	rec := &recorder{}
	c := NewClient(GUI)
	c.UID = 501
	c.Runner = rec.run
	require.NoError(t, c.Enable(context.Background(), "com.datadoghq.gui"))
	assert.Equal(t, []string{"enable", "gui/501/com.datadoghq.gui"}, rec.calls[0].args)
	assert.Equal(t, "/Library/LaunchAgents", GUI.Dir())
	assert.Equal(t, "/Library/LaunchDaemons", System.Dir())
}

func TestJobWriteIsAtomicAndLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	job := Job{Label: "com.datadoghq.agent", PlistPath: filepath.Join(dir, "com.datadoghq.agent.plist")}

	require.NoError(t, job.Write([]byte("first")))
	content, err := os.ReadFile(job.Path())
	require.NoError(t, err)
	assert.Equal(t, "first", string(content))

	// Rewriting an existing definition is what an upgrade does.
	require.NoError(t, job.Write([]byte("second")))
	content, err = os.ReadFile(job.Path())
	require.NoError(t, err)
	assert.Equal(t, "second", string(content))

	info, err := os.Stat(job.Path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "write left a temporary file behind")

	require.NoError(t, job.Remove())
	_, err = os.Stat(job.Path())
	assert.True(t, os.IsNotExist(err))
}

func TestJobPathDefaultsToTheDomainDirectory(t *testing.T) {
	assert.Equal(t, "/Library/LaunchDaemons/com.datadoghq.installer.plist",
		Job{Label: "com.datadoghq.installer", Domain: System}.Path())
	assert.Equal(t, "/Library/LaunchAgents/com.datadoghq.gui.plist",
		Job{Label: "com.datadoghq.gui", Domain: GUI}.Path())
}
