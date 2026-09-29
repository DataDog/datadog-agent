// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// wantRedacted is the placeholder these tests expect a secret to be replaced with. It deliberately
// restates the value rather than reading the production constant: a test that asserts against the
// constant it is checking passes whatever that constant is changed to. Spelled once because eight
// asterisks cannot be counted by eye, so repeating the literal invites a seven-asterisk typo.
const wantRedacted = "********"

func writeProcmgrConfigFixture(t *testing.T, root string, service MigratableService) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, service.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))
}

func reportProcessByName(t *testing.T, report SupportReport, name string) ProcessSnapshot {
	t.Helper()

	for _, process := range report.Processes {
		if process.Name == name {
			return process
		}
	}
	require.Failf(t, "missing process", "process %q was not reported", name)
	return ProcessSnapshot{}
}

func TestReportIncludesEveryProcessNotJustCatalogServices(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1, Version: "1.2.3"},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
			"some-other-process":    {Name: "some-other-process", State: ProcessStateRunning, PID: 43},
		},
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	require.Len(t, report.Processes, 2,
		"the report must cover every supervised process, not only the migratable catalog")
	assert.Equal(t, "datadog-agent-process", report.Processes[0].Name, "processes must be sorted by name")
	assert.Equal(t, "some-other-process", report.Processes[1].Name)
	assert.Equal(t, "1.2.3", report.Daemon.Version)
	assert.Empty(t, report.DaemonError)
	assert.NotEmpty(t, report.Notes, "the report must explain how to read a process that is down")
}

func TestReportUnreachableDaemonIsRecordedNotDropped(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		connectErr: errors.New("open \\\\.\\pipe\\datadog-procmgrd: file does not exist"),
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	assert.False(t, report.Daemon.Reachable)
	assert.Contains(t, report.DaemonError, "connect to dd-procmgrd",
		"a non-answering daemon must produce a report saying so, not an empty file")
	assert.NotEmpty(t, report.SocketPath, "support needs to know which endpoint was tried")
	assert.Empty(t, report.Processes)
	assert.NotEmpty(t, report.Services, "service supervision mapping does not need the daemon")
}

func TestReportStatusFailureIsRecorded(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemonErr: errors.New("deadline exceeded"),
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	assert.Contains(t, report.DaemonError, "dd-procmgrd status")
	assert.Contains(t, report.DaemonError, "deadline exceeded")
}

func TestReportListFailureKeepsDaemonState(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon:  DaemonSnapshot{Reachable: true, Ready: true},
		listErr: errors.New("list failed"),
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	assert.True(t, report.Daemon.Reachable, "a failed list must not discard the status we did get")
	assert.Empty(t, report.DaemonError)
	assert.Contains(t, report.ProcessesError, "dd-procmgrd list")
}

func TestReportDescribeFailureFallsBackToListData(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
		},
		describeErr: errors.New("describe failed"),
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	process := reportProcessByName(t, report, "datadog-agent-process")
	assert.Equal(t, uint32(42), process.PID, "a failed describe must keep the data List already gave us")
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0], "describe datadog-agent-process")
}

// A gate-blocked process and an inert catalog entry are both Created, and auto_start is the only
// field that separates them. It comes from Describe, so losing that call loses the distinction.
func TestReportDistinguishesGatedProcessFromInertCatalogEntry(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process":  {Name: "datadog-agent-process", State: ProcessStateCreated},
			"datadog-agent-sysprobe": {Name: "datadog-agent-sysprobe", State: ProcessStateCreated},
		},
		details: map[string]ProcessSnapshot{
			"datadog-agent-process": {
				Name:      "datadog-agent-process",
				State:     ProcessStateCreated,
				AutoStart: true,
			},
			"datadog-agent-sysprobe": {
				Name:      "datadog-agent-sysprobe",
				State:     ProcessStateCreated,
				AutoStart: false,
			},
		},
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	gated := reportProcessByName(t, report, "datadog-agent-process")
	assert.Equal(t, ProcessStateCreated, gated.State)
	assert.True(t, gated.AutoStart, "auto_start is what marks this as blocked rather than inert")

	inert := reportProcessByName(t, report, "datadog-agent-sysprobe")
	assert.Equal(t, ProcessStateCreated, inert.State)
	assert.False(t, inert.AutoStart)
}

func TestReportSeparatesCrashLoopFromFailedSpawn(t *testing.T) {
	exitCode := int32(2)
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"crash-looper": {
				Name:         "crash-looper",
				State:        ProcessStateCrashed,
				RestartCount: 5,
				LastExitCode: &exitCode,
			},
			"failed-spawn": {Name: "failed-spawn", State: ProcessStateFailed, RestartCount: 0},
		},
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	looper := reportProcessByName(t, report, "crash-looper")
	assert.Equal(t, uint32(5), looper.RestartCount)
	require.NotNil(t, looper.LastExitCode, "a crash loop must carry the exit code that support needs")
	assert.Equal(t, int32(2), *looper.LastExitCode)

	spawn := reportProcessByName(t, report, "failed-spawn")
	assert.Zero(t, spawn.RestartCount)
	assert.Nil(t, spawn.LastExitCode, "never having exited must stay distinct from exiting with code 0")
}

// The report is only useful to support if a Stopped legacy service can be explained, which is
// what the per-service management mode does.
func TestReportServicesExplainStoppedLegacyService(t *testing.T) {
	sysprobe, ok := serviceByID("sysprobe")
	require.True(t, ok, "system-probe must be in the migratable catalog or its flare row is silent")
	assert.Equal(t, "datadog-system-probe", sysprobe.LegacyWindowsService)

	root := t.TempDir()
	writeProcmgrConfigFixture(t, root, sysprobe)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			sysprobe.ProcmgrProcessName: {Name: sysprobe.ProcmgrProcessName, State: ProcessStateRunning},
		},
	})

	report := collector.Report(context.Background(), ScrubOptions{})

	var found bool
	for _, service := range report.Services {
		if service.ID != "sysprobe" {
			continue
		}
		found = true
		assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
		assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	}
	assert.True(t, found, "the report must carry a row for every migratable service")
}

func TestReportMarshalsToReadableJSON(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, Version: "1.2.3", UptimeSeconds: 90},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
		},
	})

	raw, err := json.Marshal(collector.Report(context.Background(), ScrubOptions{}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Contains(t, decoded, "daemon")
	assert.Contains(t, decoded, "processes")
	assert.Contains(t, decoded, "services")
	assert.Contains(t, string(raw), `"uptime_seconds":90`, "keys must be snake_case for support readability")
	assert.Contains(t, string(raw), `"management_mode"`)
}

func TestReportRedactsSecretArguments(t *testing.T) {
	windowsCommand := `C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe`
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning},
		},
		details: map[string]ProcessSnapshot{
			"datadog-agent-process": {
				Name:    "datadog-agent-process",
				State:   ProcessStateRunning,
				Command: windowsCommand,
				Args: []string{
					"--password", "separate-token-secret",
					"--api_key=inline-secret",
					"--config", `C:\Program Files\Datadog\datadog.yaml`,
				},
			},
		},
	})

	proc := reportProcessByName(t, collector.Report(context.Background(), ScrubOptions{}), "datadog-agent-process")

	// Asserted as the whole argv rather than as absences: NotContains compares whole elements, so a
	// value that leaked only part of itself, which is how this went wrong before, would satisfy it.
	assert.Equal(t, []string{
		"--password", wantRedacted,
		"--api_key=" + wantRedacted,
		"--config", `C:\Program Files\Datadog\datadog.yaml`,
	}, proc.Args, "both spellings are redacted and nothing around them is disturbed")

	// The command is never part of what gets scrubbed, so it survives intact even here, where a
	// redaction did happen. Its arguments are a different matter: see the test below.
	assert.Equal(t, windowsCommand, proc.Command, "the executable path must survive intact")
}

// Redacting one value must not disturb the arguments around it. Scrubbing the command line as a
// single joined string cost exactly this, because it re-split every element on spaces.
func TestScrubProcessArgsLeavesSurroundingArgumentsIntact(t *testing.T) {
	path := `C:\Program Files\Datadog\datadog.yaml`
	processes := []ProcessSnapshot{{Args: []string{"--password", "s3cret", "--config", path}}}

	scrubProcessArgs(processes, ScrubOptions{})

	assert.Equal(t, []string{"--password", wantRedacted, "--config", path}, processes[0].Args,
		"a path holding spaces stays one argument even when something else was redacted")
}

func TestScrubProcessArgsIsIdempotent(t *testing.T) {
	processes := []ProcessSnapshot{{Args: []string{"--password", "s3cret", "--verbose"}}}

	scrubProcessArgs(processes, ScrubOptions{})
	once := append([]string{}, processes[0].Args...)
	scrubProcessArgs(processes, ScrubOptions{})

	assert.Equal(t, once, processes[0].Args)
	assert.Equal(t, []string{"--password", wantRedacted, "--verbose"}, processes[0].Args)
}

// The shared word list spells these with underscores, but command lines just as often use hyphens,
// and a value can be attached with ":" as well as "=". Both forms slipped through a scrubber that
// matched the word list literally and split only on "=".
func TestScrubProcessArgsHandlesFlagSpellingsAndDelimiters(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "hyphenated flag with a separate value",
			args: []string{"--api-key", "leaked-by-spelling"},
			want: []string{"--api-key", wantRedacted},
		},
		{
			name: "colon delimiter keeps the value in the same token",
			args: []string{"--password:leaked-by-delimiter", "--verbose"},
			want: []string{"--password:" + wantRedacted, "--verbose"},
		},
		{
			name: "uppercase flag",
			args: []string{"--AUTH-TOKEN=leaked-by-case"},
			want: []string{"--AUTH-TOKEN=" + wantRedacted},
		},
		{
			// Everything after the flag goes, not just the first word of it. Handing the command
			// line to procutil.ScrubCommand would keep "with spaces" here.
			name: "a secret value containing spaces is redacted whole",
			args: []string{"--password", "secret with spaces"},
			want: []string{"--password", wantRedacted},
		},
		{
			// A secret flag is not always followed by its value. Redacting the next element blindly
			// overwrites the second flag, and classifying elements as they are rewritten then reads
			// the placeholder instead of "--api-key", leaving the real credential in the flare.
			name: "a secret flag following another does not lose its own value",
			args: []string{"--password", "--api-key", "leaked-by-adjacency"},
			want: []string{"--password", wantRedacted, wantRedacted},
		},
		{
			// procutil redacts this, so a flare must too: a bare name is how "key value" style
			// arguments are written, and nothing says a secret has to arrive behind a dash.
			name: "a bare secret name carries its value in the next argument",
			args: []string{"password", "leaked-by-bareness"},
			want: []string{"password", wantRedacted},
		},
		{
			// A value is not disqualified from being one by starting with a dash or a slash. Paths
			// are ordinary values, and a token can start with either.
			name: "a secret value spelled like a flag is still a value",
			args: []string{"--password", "/etc/datadog-agent/leaked-by-slash"},
			want: []string{"--password", wantRedacted},
		},
		{
			name: "a secret value starting with a dash is still a value",
			args: []string{"--api-key", "-leaked-by-dash"},
			want: []string{"--api-key", wantRedacted},
		},
		{
			// The value itself names a secret. Classifying every element before rewriting any of
			// them means this one is examined, so it must be recognized as the value it is rather
			// than as a flag whose own value needs redacting.
			name: "a value that names a secret is redacted without disturbing what follows",
			args: []string{"--password", "my-api-key-value", "--verbose"},
			want: []string{"--password", wantRedacted, "--verbose"},
		},
		{
			// Not every secret is spelled as a flag. This one carries its value on its own token,
			// so it is redacted there rather than by consuming the argument after it.
			name: "a secret named without a dash is still redacted",
			args: []string{"password=leaked-without-a-dash", "--verbose"},
			want: []string{"password=" + wantRedacted, "--verbose"},
		},
		{
			name: "a value holding a Windows path is not a flag",
			args: []string{"--config", `C:\Program Files\Datadog\datadog.yaml`},
			want: []string{"--config", `C:\Program Files\Datadog\datadog.yaml`},
		},
		{
			// procmgr reads args from a YAML list, where writing a flag and its value as one
			// entry is an easy thing to do.
			name: "flag and value in one token separated by a space",
			args: []string{"--password leaked-by-space"},
			want: []string{"--password " + wantRedacted},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processes := []ProcessSnapshot{{Args: append([]string{}, test.args...)}}
			scrubProcessArgs(processes, ScrubOptions{})
			assert.Equal(t, test.want, processes[0].Args)
		})
	}
}

// An operator who declared a word sensitive, or asked for arguments to be stripped, meant it for
// flares as well: these settings live in the Agent config, which coat does not read, so they have
// to arrive as options.
func TestScrubProcessArgsHonoursOperatorSettings(t *testing.T) {
	t.Run("a declared word matches whatever case it was written in", func(t *testing.T) {
		processes := []ProcessSnapshot{{Args: []string{"--passphrase", "operator-declared-this-secret"}}}

		scrubProcessArgs(processes, ScrubOptions{CustomSensitiveWords: []string{"PASSPHRASE"}})

		assert.Equal(t, []string{"--passphrase", wantRedacted}, processes[0].Args)
	})

	t.Run("a declared wildcard matches a prefixed flag", func(t *testing.T) {
		processes := []ProcessSnapshot{{Args: []string{"--tenant-token", "operator-declared-this-secret"}}}

		scrubProcessArgs(processes, ScrubOptions{CustomSensitiveWords: []string{"*token*"}})

		assert.Equal(t, []string{"--tenant-token", wantRedacted}, processes[0].Args,
			"wildcards are how operators declare a family of flags, so they have to be honoured")
	})

	t.Run("stripping drops every argument", func(t *testing.T) {
		processes := []ProcessSnapshot{{
			Command: "/opt/datadog-agent/embedded/bin/process-agent",
			Args:    []string{"--config", "/etc/datadog-agent/datadog.yaml"},
		}}

		scrubProcessArgs(processes, ScrubOptions{StripArguments: true})

		assert.Nil(t, processes[0].Args, "no argument should survive, not even a harmless one")
		assert.NotEmpty(t, processes[0].Command, "the executable is not an argument and stays")
	})
}

// Every leak found in this scrubber has been one instance of a single invariant: a flare must keep
// nothing procutil would have redacted. The cases above pin the shapes known to have gone wrong,
// which only ever catches the next one if somebody thinks to write it down. This asserts the
// invariant itself over a corpus of argv shapes, so a value procutil removes and this package
// leaves behind fails here whether or not anyone anticipated that spelling.
func TestRedactionKeepsNothingProcutilWouldRedact(t *testing.T) {
	corpus := [][]string{
		{"--password", "s3cret"},
		{"--password", "secret with spaces"},
		{"--password", "/etc/datadog-agent/creds"},
		{"--password", "-dash-leading-value"},
		{"--password", "--api-key", "s3cret"},
		{"--password", "my-api-key-value", "--verbose"},
		{"--api_key=inline", "--config", `C:\Program Files\Datadog\datadog.yaml`},
		{"--password:colon-delimited"},
		{"--password bundled-in-one-token"},
		{"password", "bare-name-value"},
		{"password=bare-inline"},
		{"PASSWORD", "upper-case-value"},
		{"--AUTH-TOKEN=upper-inline"},
		{"--passwd", "alias-value"},
		{"--credentials", "creds-value"},
		{"--mysql_pwd", "pwd-value"},
		{"--password"},
		{"--verbose", "--config", "/etc/datadog-agent/datadog.yaml"},
	}

	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(slices.Clone(hyphenSpelledSecretWords))

	for _, args := range corpus {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			// procutil's own verdict on the same command line, used only to decide which text is
			// secret. Its re-splitting is why this package does not use it to redact.
			byProcutil, _ := scrubber.ScrubCommand(append([]string{"agent"}, args...))
			procutilOutput := strings.Join(byProcutil, " ")

			processes := []ProcessSnapshot{{Args: slices.Clone(args)}}
			scrubProcessArgs(processes, ScrubOptions{})
			ourOutput := strings.Join(processes[0].Args, " ")

			for _, token := range args {
				if strings.Contains(procutilOutput, token) {
					continue // procutil kept it, so it is not a secret and we may keep it too
				}
				assert.NotContains(t, ourOutput, token,
					"procutil redacted %q out of this command line, so the flare must not carry it", token)
			}
		})
	}
}

// namesSecret probes procutil's patterns with a synthetic "<flag>=x", which assumes the shape of the
// regexes procutil compiles. Nothing in procutil promises that shape. If it changed, the probe would
// stop matching, every flag would look harmless and secrets would reach flares with all the tests
// above still green. Cross-check the two so that change breaks the build instead, and confirm the
// placeholder we write is the one procutil substitutes, since procutil does not export it.
func TestNamesSecretAgreesWithProcutil(t *testing.T) {
	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(slices.Clone(hyphenSpelledSecretWords))

	for _, flag := range []string{
		"--password", "--api_key", "--api-key", "--auth_token", "--AUTH-TOKEN",
		"--config", "--verbose", "--sysprobe-config",
	} {
		t.Run(flag, func(t *testing.T) {
			scrubbed, procutilRedacted := scrubber.ScrubCommand([]string{"agent", flag, "a-value"})

			assert.Equal(t, procutilRedacted, namesSecret(scrubber.SensitivePatterns, flag),
				"our decision about this flag must match what procutil itself would redact")

			if procutilRedacted {
				assert.Contains(t, scrubbed, wantRedacted,
					"procutil's placeholder must still be the one this package writes")
			}
		})
	}
}

// The operator's settings have to reach Report, not be applied to what it returns. Redacting a value
// overwrites the argument that held it, so a pass with only the default words leaves a placeholder
// where "--tenant-thing" was, and no later pass can recognize it as a name the operator declared
// sensitive. A flare then ships the value.
func TestReportHonoursOperatorWordsBesideDefaultOnes(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning},
		},
		details: map[string]ProcessSnapshot{
			"datadog-agent-process": {
				Name:  "datadog-agent-process",
				State: ProcessStateRunning,
				Args:  []string{"--password", "--tenant-thing", "leaked-beside-a-default-flag"},
			},
		},
	})

	report := collector.Report(context.Background(), ScrubOptions{CustomSensitiveWords: []string{"*tenant*"}})

	proc := reportProcessByName(t, report, "datadog-agent-process")
	assert.NotContains(t, strings.Join(proc.Args, " "), "leaked-beside-a-default-flag",
		"a declared word must be honoured even when a default-word flag precedes it")
}
