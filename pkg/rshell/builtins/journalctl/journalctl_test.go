// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package journalctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type fakeJournalReader struct {
	queries []builtins.JournalQuery
	entries []builtins.JournalEntry
	err     error
}

type fakeJournalStorage struct {
	usage builtins.JournalUsage
	err   error
	calls int
}

type fakeJournalCleaner struct {
	result   builtins.JournalVacuumResult
	err      error
	requests []builtins.JournalVacuumRequest
	order    *[]string
}

type fakeJournalRotator struct {
	err   error
	calls int
	order *[]string
}

func (c *fakeJournalCleaner) VacuumJournal(_ context.Context, request builtins.JournalVacuumRequest) (builtins.JournalVacuumResult, error) {
	if c.order != nil {
		*c.order = append(*c.order, "vacuum")
	}
	c.requests = append(c.requests, request)
	return c.result, c.err
}

func (r *fakeJournalRotator) RotateJournal(context.Context) error {
	if r.order != nil {
		*r.order = append(*r.order, "rotate")
	}
	r.calls++
	return r.err
}

func (s *fakeJournalStorage) JournalDiskUsage(context.Context) (builtins.JournalUsage, error) {
	s.calls++
	return s.usage, s.err
}

func (r *fakeJournalReader) ReadJournal(_ context.Context, query builtins.JournalQuery, yield func(builtins.JournalEntry) error) error {
	r.queries = append(r.queries, query)
	for _, entry := range r.entries {
		if err := yield(entry); err != nil {
			return err
		}
	}
	return r.err
}

func runJournalctl(t *testing.T, args []string, callCtx *builtins.CallContext) builtins.Result {
	t.Helper()
	fs := pflag.NewFlagSet("journalctl", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := makeFlags(fs)
	require.NoError(t, fs.Parse(args))
	return handler(context.Background(), callCtx, fs.Args())
}

func TestJournalctlBuildsBoundedUnitQuery(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	reader := &fakeJournalReader{entries: []builtins.JournalEntry{{Message: "ready"}}}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	callCtx := &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    now,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append([]builtins.SystemdOperation(nil), operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	}

	result := runJournalctl(t, []string{
		"-u", "api.service",
		"-u", "worker.service",
		"-u", "api.service",
		"-b",
		"-n", "2",
		"--since", "15m",
		"--output", "cat",
	}, callCtx)

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "ready\n", stdout.String())
	assert.Equal(t, []builtins.SystemdOperation{
		{Service: "api.service", Action: builtins.SystemServiceRead},
		{Service: "worker.service", Action: builtins.SystemServiceRead},
	}, authorized)
	require.Len(t, reader.queries, 1)
	assert.Equal(t, builtins.JournalQuery{
		Units:       []string{"api.service", "worker.service"},
		CurrentBoot: true,
		Since:       now.Add(-15 * time.Minute),
		MaxEntries:  2,
	}, reader.queries[0])
}

func TestJournalctlShortOutputEscapesTerminalControls(t *testing.T) {
	reader := &fakeJournalReader{entries: []builtins.JournalEntry{{
		Timestamp:  time.Date(2026, time.July, 14, 12, 34, 56, 0, time.UTC),
		Hostname:   "host\tname",
		Identifier: "kernel\x1b",
		PID:        "\xff",
		Message:    "cafe\u0301\nline\x00\u202e",
	}}}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	result := runJournalctl(t, []string{"-k", "-n1", "--output", "short"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append(authorized, operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Jul 14 12:34:56 host\\tname kernel\\x1b[\\xff]: cafe\u0301\\nline\\x00\\u202e\n", stdout.String())
	assert.Equal(t, []builtins.SystemdOperation{{
		Service: builtins.SystemdJournaldService,
		Action:  builtins.SystemServiceRead,
	}}, authorized)
	require.Len(t, reader.queries, 1)
	assert.True(t, reader.queries[0].Kernel)
	assert.True(t, reader.queries[0].CurrentBoot)
}

func TestJournalctlShortOutputLabelsUnidentifiedKernelEntries(t *testing.T) {
	reader := &fakeJournalReader{entries: []builtins.JournalEntry{{
		Timestamp: time.Date(2026, time.July, 14, 12, 34, 56, 0, time.UTC),
		Message:   "booting",
	}}}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"-k"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Jul 14 12:34:56 - kernel: booting\n", stdout.String())
	require.Len(t, reader.queries, 1)
	assert.True(t, reader.queries[0].Kernel)
}

func TestJournalctlCatOutputPreservesRawMessageBytes(t *testing.T) {
	message := "first\nsecond\t\x1b[31mred\x1b[0m\x00\xff\n"
	reader := &fakeJournalReader{entries: []builtins.JournalEntry{{Message: message}}}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"-k", "--output", "cat"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, []byte(message+"\n"), stdout.Bytes())
	require.Len(t, reader.queries, 1)
}

func TestJournalctlDiskUsageUsesStorageReadCapability(t *testing.T) {
	storage := &fakeJournalStorage{usage: builtins.JournalUsage{Bytes: 5 * 1024 * 1024, Files: 3}}
	reader := &fakeJournalReader{}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	result := runJournalctl(t, []string{"--disk-usage"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append(authorized, operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader, JournalStorage: storage},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Archived and active journals take up 5.0M in the file system.\n", stdout.String())
	assert.Equal(t, []builtins.SystemdOperation{{
		Service: builtins.SystemdJournaldService,
		Action:  builtins.SystemServiceRead,
	}}, authorized)
	assert.Equal(t, 1, storage.calls)
	assert.Empty(t, reader.queries)
}

func TestJournalctlDiskUsageIsExclusive(t *testing.T) {
	for _, args := range [][]string{
		{"--disk-usage", "-u", "api.service"},
		{"--disk-usage", "-k"},
		{"--disk-usage", "-b"},
		{"--disk-usage", "-n", "10"},
		{"--disk-usage", "--since", "1h"},
		{"--disk-usage", "-o", "cat"},
		{"--disk-usage", "--rotate"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			storage := &fakeJournalStorage{}
			var stdout, stderr bytes.Buffer
			result := runJournalctl(t, args, &builtins.CallContext{
				Stdout:  &stdout,
				Stderr:  &stderr,
				Systemd: &builtins.SystemdServices{JournalStorage: storage},
			})
			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "cannot be combined")
			assert.Zero(t, storage.calls)
		})
	}
}

func TestJournalctlRotateUsesStorageCleanCapability(t *testing.T) {
	rotator := &fakeJournalRotator{}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	result := runJournalctl(t, []string{"--rotate"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append(authorized, operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalRotator: rotator},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Journal rotation completed.\n", stdout.String())
	assert.Equal(t, []builtins.SystemdOperation{{
		Service: builtins.SystemdJournaldService,
		Action:  builtins.SystemServiceClean,
	}}, authorized)
	assert.Equal(t, 1, rotator.calls)
}

func TestJournalctlRotateRunsBeforeVacuum(t *testing.T) {
	order := []string{}
	rotator := &fakeJournalRotator{order: &order}
	cleaner := &fakeJournalCleaner{order: &order, result: builtins.JournalVacuumResult{Files: 1, Bytes: 1024}}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--rotate", "--vacuum-time", "1h"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalRotator: rotator, JournalCleaner: cleaner},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, []string{"rotate", "vacuum"}, order)
	assert.Equal(t, "Journal rotation completed.\nVacuuming done, freed 1.0K from 1 archived journal file.\n", stdout.String())
}

func TestJournalctlRotationFailurePreventsVacuum(t *testing.T) {
	rotator := &fakeJournalRotator{err: errors.New("rotation failed")}
	cleaner := &fakeJournalCleaner{}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--rotate", "--vacuum-time", "1h"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalRotator: rotator, JournalCleaner: cleaner},
	})

	assert.Equal(t, uint8(1), result.Code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "rotation failed")
	assert.Equal(t, 1, rotator.calls)
	assert.Empty(t, cleaner.requests)
}

func TestJournalctlCombinedMaintenanceChecksCapabilitiesBeforeMutation(t *testing.T) {
	t.Run("missing cleaner", func(t *testing.T) {
		rotator := &fakeJournalRotator{}
		var stdout, stderr bytes.Buffer
		result := runJournalctl(t, []string{"--rotate", "--vacuum-time", "1h"}, &builtins.CallContext{
			Stdout: &stdout,
			Stderr: &stderr,
			Now:    time.Now(),
			AuthorizeSystemd: func(...builtins.SystemdOperation) error {
				return nil
			},
			Systemd: &builtins.SystemdServices{JournalRotator: rotator},
		})
		assert.Equal(t, uint8(1), result.Code)
		assert.Contains(t, stderr.String(), "cleaner capability is not available")
		assert.Zero(t, rotator.calls)
	})

	t.Run("missing rotator", func(t *testing.T) {
		cleaner := &fakeJournalCleaner{}
		var stdout, stderr bytes.Buffer
		result := runJournalctl(t, []string{"--rotate", "--vacuum-time", "1h"}, &builtins.CallContext{
			Stdout: &stdout,
			Stderr: &stderr,
			Now:    time.Now(),
			AuthorizeSystemd: func(...builtins.SystemdOperation) error {
				return nil
			},
			Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
		})
		assert.Equal(t, uint8(1), result.Code)
		assert.Contains(t, stderr.String(), "rotation capability is not available")
		assert.Empty(t, cleaner.requests)
	})
}

func TestJournalctlVacuumAuthorizesCleanAndBuildsRequest(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	cleaner := &fakeJournalCleaner{result: builtins.JournalVacuumResult{Files: 2, Bytes: 3 * 1024 * 1024}}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	result := runJournalctl(t, []string{"--vacuum-size", "64M", "--vacuum-time", "168h", "--dry-run"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    now,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append(authorized, operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Vacuuming would free 3.0M from 2 archived journal files.\n", stdout.String())
	assert.Equal(t, []builtins.SystemdOperation{{
		Service: builtins.SystemdJournaldService,
		Action:  builtins.SystemServiceClean,
	}}, authorized)
	require.Len(t, cleaner.requests, 1)
	assert.Equal(t, builtins.JournalVacuumRequest{
		Now:      now,
		Before:   now.Add(-168 * time.Hour),
		MaxBytes: 64 * 1024 * 1024,
		DryRun:   true,
	}, cleaner.requests[0])
}

func TestJournalctlVacuumFormatsSingularResult(t *testing.T) {
	cleaner := &fakeJournalCleaner{result: builtins.JournalVacuumResult{Files: 1, Bytes: 1024}}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--vacuum-time", "1h"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
	})
	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Vacuuming done, freed 1.0K from 1 archived journal file.\n", stdout.String())
}

func TestJournalctlVacuumReportsRemainingAllocationAboveSizeTarget(t *testing.T) {
	cleaner := &fakeJournalCleaner{result: builtins.JournalVacuumResult{
		Files:          1,
		Bytes:          1024,
		RemainingBytes: 96 * 1024 * 1024,
	}}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--vacuum-size", "64M", "--vacuum-time", "1h"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
	})
	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Vacuuming done, freed 1.0K from 1 archived journal file.\n"+
		"Archived and active journals still take up 96.0M, above the requested 64.0M; only archives older than --vacuum-time can be removed.\n",
		stdout.String())
}

func TestJournalctlVacuumSizeRequiresTimeFloorBeforeAuthorization(t *testing.T) {
	cleaner := &fakeJournalCleaner{}
	var stdout, stderr bytes.Buffer
	authorized := 0
	result := runJournalctl(t, []string{"--vacuum-size", "64M"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			authorized++
			return nil
		},
		Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
	})

	assert.Equal(t, uint8(1), result.Code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "journalctl: --vacuum-size requires --vacuum-time to protect recently modified archives\n", stderr.String())
	assert.Zero(t, authorized)
	assert.Empty(t, cleaner.requests)
}

func TestJournalctlMaintenanceRejectsInvalidOrMixedOptionsBeforeAuthorization(t *testing.T) {
	tests := [][]string{
		{"--dry-run"},
		{"--vacuum-size", "0"},
		{"--vacuum-size", "-1"},
		{"--vacuum-time", "0s"},
		{"--vacuum-time", "-1h"},
		{"--vacuum-time", "7d"},
		{"--rotate", "--dry-run", "--vacuum-time", "1h"},
		{"--rotate", "-u", "api.service"},
		{"--vacuum-size", "1M", "-u", "api.service"},
		{"--vacuum-time", "1h", "-k"},
		{"--vacuum-size", "1M", "-n", "10"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			cleaner := &fakeJournalCleaner{}
			var stdout, stderr bytes.Buffer
			authorized := 0
			result := runJournalctl(t, args, &builtins.CallContext{
				Stdout: &stdout,
				Stderr: &stderr,
				Now:    time.Now(),
				AuthorizeSystemd: func(...builtins.SystemdOperation) error {
					authorized++
					return nil
				},
				Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
			})
			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.NotEmpty(t, stderr.String())
			assert.Zero(t, authorized)
			assert.Empty(t, cleaner.requests)
		})
	}
}

func TestJournalctlRotateDenialPreventsMutation(t *testing.T) {
	rotator := &fakeJournalRotator{}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--rotate"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return errors.New("clean denied")
		},
		Systemd: &builtins.SystemdServices{JournalRotator: rotator},
	})
	assert.Equal(t, uint8(1), result.Code)
	assert.Contains(t, stderr.String(), "clean denied")
	assert.Zero(t, rotator.calls)
}

func TestJournalctlVacuumDenialPreventsCleanup(t *testing.T) {
	cleaner := &fakeJournalCleaner{}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--vacuum-time", "1h"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return errors.New("clean denied")
		},
		Systemd: &builtins.SystemdServices{JournalCleaner: cleaner},
	})
	assert.Equal(t, uint8(1), result.Code)
	assert.Contains(t, stderr.String(), "clean denied")
	assert.Empty(t, cleaner.requests)
}

func TestFormatUsage(t *testing.T) {
	assert.Equal(t, "0B", formatUsage(0))
	assert.Equal(t, "1023B", formatUsage(1023))
	assert.Equal(t, "1.0K", formatUsage(1024))
	assert.Equal(t, "1.5M", formatUsage(1536*1024))
}

func TestJournalctlAuthorizesEveryScopeBeforeReading(t *testing.T) {
	reader := &fakeJournalReader{}
	var stdout, stderr bytes.Buffer
	authorizationErr := errors.New("worker.service is denied")
	result := runJournalctl(t, []string{"-u", "api.service", "-u", "worker.service"}, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			require.Len(t, operations, 2)
			return authorizationErr
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})

	assert.Equal(t, uint8(1), result.Code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), authorizationErr.Error())
	assert.Empty(t, reader.queries)
}

func TestJournalctlRejectsUnboundedInputsBeforeAuthorization(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing scope"},
		{name: "mixed scopes", args: []string{"-u", "api.service", "-k"}},
		{name: "from head", args: []string{"-u", "api.service", "-n", "+10"}},
		{name: "negative lines", args: []string{"-u", "api.service", "--lines=-1"}},
		{name: "too many lines", args: []string{"-u", "api.service", "-n", "1001"}},
		{name: "structured output", args: []string{"-u", "api.service", "-o", "json"}},
		{name: "raw match", args: []string{"-u", "api.service", "_PID=1"}},
		{name: "bad since", args: []string{"-u", "api.service", "--since", "yesterday"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeJournalReader{}
			var stdout, stderr bytes.Buffer
			authorizeCalls := 0
			result := runJournalctl(t, test.args, &builtins.CallContext{
				Stdout: &stdout,
				Stderr: &stderr,
				Now:    time.Now(),
				AuthorizeSystemd: func(...builtins.SystemdOperation) error {
					authorizeCalls++
					return nil
				},
				Systemd: &builtins.SystemdServices{Journal: reader},
			})

			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.NotEmpty(t, stderr.String())
			assert.Zero(t, authorizeCalls)
			assert.Empty(t, reader.queries)
		})
	}
}

func TestJournalctlRejectsDangerousJournalctlOptions(t *testing.T) {
	Cmd.Register()
	handler, ok := builtins.Lookup("journalctl")
	require.True(t, ok)

	for _, args := range [][]string{
		{"--follow"},
		{"--file=/tmp/system.journal"},
		{"--directory=/var/log/journal"},
		{"--root=/host"},
		{"--machine=host"},
		{"--namespace=tenant"},
		{"--cursor=cursor"},
		{"--grep=secret"},
		{"--reverse"},
		{"--all"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			result := handler(context.Background(), &builtins.CallContext{
				Stdout: &stdout,
				Stderr: &stderr,
			}, args)
			assert.Equal(t, uint8(1), result.Code)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "unrecognized option")
		})
	}
}

func TestJournalctlDeduplicatesRepeatedUnitScopesBeforeLimit(t *testing.T) {
	args := make([]string, 0, (builtins.MaxJournalQueryUnits+1)*2)
	for i := 0; i <= builtins.MaxJournalQueryUnits; i++ {
		args = append(args, "-u", "api.service")
	}
	reader := &fakeJournalReader{}
	var stdout, stderr bytes.Buffer
	var authorized []builtins.SystemdOperation
	result := runJournalctl(t, args, &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		AuthorizeSystemd: func(operations ...builtins.SystemdOperation) error {
			authorized = append(authorized, operations...)
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})

	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, []builtins.SystemdOperation{{Service: "api.service", Action: builtins.SystemServiceRead}}, authorized)
	require.Len(t, reader.queries, 1)
	assert.Equal(t, []string{"api.service"}, reader.queries[0].Units)
}

func TestJournalctlLimitsUniqueUnitScopes(t *testing.T) {
	args := make([]string, 0, (builtins.MaxJournalQueryUnits+1)*2)
	for i := 0; i <= builtins.MaxJournalQueryUnits; i++ {
		args = append(args, "-u", strings.Repeat("a", i+1)+".service")
	}
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, args, &builtins.CallContext{Stdout: &stdout, Stderr: &stderr})

	assert.Equal(t, uint8(1), result.Code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "too many unit scopes")
}

func TestJournalctlHelpDoesNotRequireSystemdCapability(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := runJournalctl(t, []string{"--help"}, &builtins.CallContext{Stdout: &stdout, Stderr: &stderr})
	assert.Equal(t, uint8(0), result.Code)
	assert.Contains(t, stdout.String(), "Usage: journalctl")
	assert.Contains(t, stdout.String(), "--unit")
	assert.Contains(t, stdout.String(), "--rotate")
	assert.Contains(t, stdout.String(), "--vacuum-time")
	assert.Contains(t, stdout.String(), "output format: escaped short or raw cat")
	assert.Contains(t, stdout.String(), "alone, remove archives older than AGE; with --vacuum-size, set the minimum deletion age")
	assert.Empty(t, stderr.String())
}

type brokenPipeWriter struct{}

func (brokenPipeWriter) Write([]byte) (int, error) {
	return 0, syscall.EPIPE
}

func TestJournalctlTreatsBrokenPipeAsSuccess(t *testing.T) {
	reader := &fakeJournalReader{entries: []builtins.JournalEntry{{Message: "entry"}}}
	var stderr bytes.Buffer
	result := runJournalctl(t, []string{"-k"}, &builtins.CallContext{
		Stdout: brokenPipeWriter{},
		Stderr: &stderr,
		AuthorizeSystemd: func(...builtins.SystemdOperation) error {
			return nil
		},
		Systemd: &builtins.SystemdServices{Journal: reader},
	})
	assert.Equal(t, uint8(0), result.Code)
	assert.Empty(t, stderr.String())
}

func TestParseSince(t *testing.T) {
	location := time.FixedZone("test", -4*60*60)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, location)

	absolute, ok := parseSince("2026-07-14T15:00:00Z", now)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, time.July, 14, 15, 0, 0, 0, time.UTC), absolute)

	local, ok := parseSince("2026-07-14 10:30:00", now)
	require.True(t, ok)
	assert.Equal(t, time.Date(2026, time.July, 14, 10, 30, 0, 0, location), local)

	lookback, ok := parseSince("90m", now)
	require.True(t, ok)
	assert.Equal(t, now.Add(-90*time.Minute), lookback)

	_, ok = parseSince("-1h", now)
	assert.False(t, ok)
	_, ok = parseSince(strings.Repeat("9", 100), now)
	assert.False(t, ok)
}
