// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package testutil

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunJavaVersion run class under java version
func RunJavaVersion(t testing.TB, version, class string, waitForParam ...*regexp.Regexp) error {
	t.Helper()
	var waitFor *regexp.Regexp
	if len(waitForParam) == 0 {
		// test if injection happen
		waitFor = regexp.MustCompile(`loading TestAgentLoaded\.agentmain.*`)
	} else {
		waitFor = waitForParam[0]
	}

	dir, _ := testutil.CurDir()
	addr := "172.17.0.1" // for some reason docker network inspect bridge --format='{{(index .IPAM.Config 0).Gateway}}'   is not reliable and doesn't report Gateway ip sometime
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
		"EXTRA_HOSTS=host.docker.internal:" + addr,
	}

	return protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, waitFor, protocolsUtils.DefaultTimeout)
}

// FindProcessByCommandLine gets a proc name and part of its command line, and returns all PIDs matched for those conditions.
func FindProcessByCommandLine(procName, command string) ([]int, error) {
	res := make([]int, 0)
	fn := func(pid int) error {
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			return nil // ignore process that didn't exist anymore
		}

		name, err := proc.Name()
		if err == nil && name == procName {
			cmdline, err := proc.Cmdline()
			if err == nil && strings.Contains(cmdline, command) {
				res = append(res, pid)
			}
		}
		return nil
	}

	if err := kernel.WithAllProcs(kernel.ProcFSRoot(), fn); err != nil {
		return nil, err
	}
	return res, nil
}

// RunJavaVersionAndWaitForRejection is running a java version program, waiting for it to successfully load, and then
// checking the java TLS program didn't attach to it (rejected the injection). The last part is done using log scanner
// we're registering a new log scanner and looking for a specific log (java pid (\d+) attachment rejected).
func RunJavaVersionAndWaitForRejection(t testing.TB, version, class string, waitForCondition *regexp.Regexp) {
	t.Helper()

	dir, _ := testutil.CurDir()
	addr := "172.17.0.1" // for some reason docker network inspect bridge --format='{{(index .IPAM.Config 0).Gateway}}'   is not reliable and doesn't report Gateway ip sometime
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
		"EXTRA_HOSTS=host.docker.internal:" + addr,
	}

	l := javaInjectionRejectionLogger{
		t:            t,
		lock:         sync.RWMutex{},
		rejectedPIDs: make(map[int32]struct{}),
	}
	configureLoggerForTest(t, &l)

	require.NoError(t, protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, waitForCondition, time.Second*15))
	pids, err := FindProcessByCommandLine("java", class)
	require.NoError(t, err)
	require.Lenf(t, pids, 1, "found more process (%d) than expected (1)", len(pids))
	require.Eventuallyf(t, func() bool {
		return l.HasRejectedPID(int32(pids[0]))
	}, time.Second*30, time.Millisecond*100, "pid %d was not rejected", pids[0])
}

func configureLoggerForTest(t testing.TB, w io.Writer) func() {
	logger, err := seelog.LoggerFromWriterWithMinLevel(w, seelog.TraceLvl)
	if err != nil {
		t.Fatalf("unable to configure logger, err: %v", err)
	}
	seelog.ReplaceLogger(logger) //nolint:errcheck
	log.SetupLogger(logger, "debug")
	return log.Flush
}

type javaInjectionRejectionLogger struct {
	t            testing.TB
	lock         sync.RWMutex
	rejectedPIDs map[int32]struct{}
}

var (
	rejectionRegex = regexp.MustCompile(`java pid (\d+) attachment rejected`)
)

// Write implements the io.Writer interface.
func (l *javaInjectionRejectionLogger) Write(p []byte) (n int, err error) {
	res := rejectionRegex.FindAllSubmatch(p, -1)
	// We expect to have 1 group (len(res) == 1) and the match (res[0]) should have 2 entries, the first is the full string
	// and the second (res[0][1]) is the group (the PID).
	if len(res) == 1 && len(res[0]) == 2 {
		i, err := strconv.Atoi(string(res[0][1]))
		if err == nil {
			l.lock.Lock()
			l.rejectedPIDs[int32(i)] = struct{}{}
			l.lock.Unlock()
		}
	}
	return len(p), nil
}

// HasRejectedPID returns true if the given pid was rejected by java tls.
func (l *javaInjectionRejectionLogger) HasRejectedPID(pid int32) bool {
	l.lock.RLock()
	defer l.lock.RUnlock()
	_, ok := l.rejectedPIDs[pid]
	return ok
}
