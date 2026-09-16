// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var _ = declareInlineConfig(TestReplay)

func TestReplay(t *testing.T) {
	SkipIfNotAvailable(t)

	t.Run("host-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay_host",
				Expression: `exec.comm in ["testsuite"]`,
			},
		}

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_host")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)

				// validate that pid 1 is reported as an exec
				ancestor := e.ProcessContext.Ancestor
				for ancestor != nil {
					if ancestor.Pid == 1 && !ancestor.IsExec {
						t.Errorf("pid1 should be reported as an Exec: %+v", e)
					}
					ancestor = ancestor.Ancestor
				}

				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from snapshot")
	})

	t.Run("process-credentials", func(t *testing.T) {
		if ebpfLessEnabled {
			t.Skip("requires the eBPF process snapshot")
		}
		if testEnvironment == DockerEnvironment || utils.Getpid() != uint32(os.Getpid()) {
			t.Skip("requires the test process PID to be visible through the host procfs")
		}
		if os.Geteuid() != 0 {
			t.Skip("requires root to set process credentials")
		}

		const (
			uid   = 42401
			euid  = 0
			suid  = 42403
			fsuid = 42404
			gid   = 42501
			egid  = 0
			sgid  = 42503
			fsgid = 42504
		)
		// All values in each status line are distinct, so an incorrect index
		// in the UID/GID mapping cannot pass this test.

		// Start this process before the module so it can only enter the cache
		// through EBPFResolvers.Snapshot -> SyncCache -> enrichEventFromProcfs.
		syscallTester, err := syscallTesterFS.ReadFile("syscall_tester/bin/syscall_tester")
		if err != nil {
			t.Fatal(err)
		}
		snapshotTester := filepath.Join(t.TempDir(), "syscall_tester")
		if err := os.WriteFile(snapshotTester, syscallTester, 0o700); err != nil {
			t.Fatal(err)
		}

		snapshotArgs := []string{
			"snapshot-credentials",
			strconv.Itoa(uid), strconv.Itoa(euid), strconv.Itoa(suid), strconv.Itoa(fsuid),
			strconv.Itoa(gid), strconv.Itoa(egid), strconv.Itoa(sgid), strconv.Itoa(fsgid),
		}
		cmd := exec.Command(snapshotTester, snapshotArgs...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})

		var expectedCredentials model.Credentials
		if !assert.Eventually(t, func() bool {
			var err error
			expectedCredentials, err = readProcSnapshotCredentials(uint32(cmd.Process.Pid))
			return err == nil &&
				expectedCredentials.UID == uid && expectedCredentials.EUID == euid && expectedCredentials.FSUID == fsuid &&
				expectedCredentials.GID == gid && expectedCredentials.EGID == egid && expectedCredentials.FSGID == fsgid
		}, time.Second, 10*time.Millisecond, "the credential test process was not configured") {
			return
		}

		expectedArgs := append([]string{snapshotTester}, snapshotArgs...)

		fileInfo, err := os.Stat(snapshotTester)
		if err != nil {
			t.Fatal(err)
		}
		fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatalf("unexpected stat type %T", fileInfo.Sys())
		}

		ruleDefs := []*rules.RuleDefinition{
			{
				ID: "test_rule_replay_process_credentials",
				Expression: fmt.Sprintf(`event.source == "replay" && exec.pid == %d && exec.comm == "syscall_tester" &&
					process.uid == %d && process.euid == %d && process.fsuid == %d &&
					process.gid == %d && process.egid == %d && process.fsgid == %d`,
					cmd.Process.Pid, uid, euid, fsuid, gid, egid, fsgid),
			},
		}

		gotEvent := atomic.NewBool(false)
		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_process_credentials")
				assert.Equal(t, model.EventSourceReplay, e.Source)
				assert.True(t, e.IsEventFromReplay())
				assert.Equal(t, uint32(model.ExecEventType), e.Type)
				assert.NotNil(t, e.ProcessCacheEntry)
				assert.Equal(t, uint64(model.ProcessCacheEntryFromSnapshot), e.ProcessContext.Source)
				assert.True(t, e.ProcessCacheEntry.IsExec)
				assert.Equal(t, uint32(cmd.Process.Pid), e.ProcessContext.Pid)
				assert.Equal(t, uint32(cmd.Process.Pid), e.ProcessContext.Tid)
				assert.Equal(t, uint32(os.Getpid()), e.ProcessContext.PPid)
				assert.Equal(t, snapshotTester, e.Exec.FileEvent.PathnameStr)
				assert.Equal(t, model.MountOriginProcfs, e.Exec.FileEvent.MountOrigin)
				assert.Equal(t, model.MountSourceSnapshot, e.Exec.FileEvent.MountSource)
				assert.Equal(t, uint64(fileStat.Ino), e.Exec.FileEvent.Inode)
				assert.Equal(t, uint32(fileStat.Uid), e.Exec.FileEvent.UID)
				assert.Equal(t, uint32(fileStat.Gid), e.Exec.FileEvent.GID)
				assertRights(t, e.Exec.FileEvent.Mode, uint16(fileStat.Mode)&0o1777)
				assert.Equal(t, expectedArgs, e.Exec.ArgsEntry.Values)
				assert.True(t, e.Exec.ForkTime.Equal(e.Exec.ExecTime))

				expectedAUID := expectedCredentials.AUID
				if parent := e.ProcessCacheEntry.Parent; parent != nil {
					parentAUID := parent.Credentials.AUID
					// Process-cache lineage currently treats an AUID of zero as
					// missing. Consequently, SetExecParent replaces a raw zero read
					// from procfs with its parent's AUID.
					if expectedAUID == 0 {
						expectedAUID = parentAUID
					}
				}
				assertFieldEqual(t, e, "process.uid", int(expectedCredentials.UID))
				assertFieldEqual(t, e, "process.euid", int(expectedCredentials.EUID))
				assertFieldEqual(t, e, "process.fsuid", int(expectedCredentials.FSUID))
				assertFieldEqual(t, e, "process.gid", int(expectedCredentials.GID))
				assertFieldEqual(t, e, "process.egid", int(expectedCredentials.EGID))
				assertFieldEqual(t, e, "process.fsgid", int(expectedCredentials.FSGID))
				assertFieldEqual(t, e, "process.auid", int(expectedAUID))
				assertFieldEqual(t, e, "process.cap_effective", int(expectedCredentials.CapEffective))
				assertFieldEqual(t, e, "process.cap_permitted", int(expectedCredentials.CapPermitted))

				assert.Equal(t, expectedCredentials.UID, e.Exec.Credentials.UID)
				assert.Equal(t, expectedCredentials.EUID, e.Exec.Credentials.EUID)
				assert.Equal(t, expectedCredentials.FSUID, e.Exec.Credentials.FSUID)
				assert.Equal(t, expectedCredentials.GID, e.Exec.Credentials.GID)
				assert.Equal(t, expectedCredentials.EGID, e.Exec.Credentials.EGID)
				assert.Equal(t, expectedCredentials.FSGID, e.Exec.Credentials.FSGID)
				assert.Equal(t, expectedAUID, e.Exec.Credentials.AUID)
				assert.Equal(t, expectedCredentials.CapEffective, e.Exec.Credentials.CapEffective)
				assert.Equal(t, expectedCredentials.CapPermitted, e.Exec.Credentials.CapPermitted)

				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)
				gotEvent.Store(true)
			},
		}))
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from the process snapshot")
	})

	t.Run("container-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay_container",
				Expression: `exec.comm in ["sleep"] && process.argv in ["123"] && process.container.id != ""`,
			},
		}

		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		dockerWrapper, err := newDockerCmdWrapper("/tmp", "/tmp", "ubuntu", "")
		if err != nil {
			t.Fatalf("failed to create docker wrapper: %v", err)
		}

		if _, err := dockerWrapper.start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			output, err := dockerWrapper.stop()
			if err != nil {
				t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
			}
		})

		sleepCtx, cancel := context.WithCancel(context.Background())

		go func() {
			cmd := dockerWrapper.CommandContext(sleepCtx, "sh", []string{"-c", "sleep 123"}, nil)
			_ = cmd.Run()
		}()

		// wait a bit so that the command is running and captured by the snapshot
		time.Sleep(2 * time.Second)

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_container")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)
				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		// make sure the cancel happens before the test module is closed
		defer cancel()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from snapshot")
	})

	t.Run("replay-event", func(t *testing.T) {
		ruleDefs := []*rules.RuleDefinition{
			{
				ID:         "test_rule_replay",
				Expression: `event.source == "replay" && exec.comm in ["sleep"]`,
			},
		}

		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		dockerWrapper, err := newDockerCmdWrapper("/tmp", "/tmp", "ubuntu", "")
		if err != nil {
			t.Fatalf("failed to create docker wrapper: %v", err)
		}

		if _, err := dockerWrapper.start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			output, err := dockerWrapper.stop()
			if err != nil {
				t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
			}
		})

		var cmd *exec.Cmd
		go func() {
			cmd = dockerWrapper.Command("sh", []string{"-c", "sleep 123"}, nil)
			_ = cmd.Run()
		}()

		// wait a bit so that the command is running and captured by the snapshot
		time.Sleep(2 * time.Second)

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(testMod *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay")
				testMod.validateExecSchema(t, e)
				validateProcessContext(t, e)
				gotEvent.Store(true)
			},
		}))

		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()
		defer cmd.Cancel()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the event from replay")
	})

	t.Run("procfs-snapshot-load-module", func(t *testing.T) {
		if testEnvironment == DockerEnvironment {
			t.Skip("skipping kernel module snapshot test in docker")
		}

		// Make sure the module is loaded before the agent starts so that the
		// snapshot path picks it up. testModuleName is defined in
		// kernel_module_test.go (same package) and points at cifs by default.
		if _, err := loadModule(testModuleName); err != nil {
			t.Skipf("failed to load %s module: %v", testModuleName, err)
		}

		ruleDefs := []*rules.RuleDefinition{
			{
				ID: "test_rule_replay_load_module",
				Expression: fmt.Sprintf(
					`event.source == "replay" && load_module.name == "%s" && load_module.loaded_from_memory == false`,
					testModuleName,
				),
			},
		}

		gotEvent := atomic.NewBool(false)

		test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
			ruleMatchHandler: func(_ *testModule, e *model.Event, r *rules.Rule) {
				assertTriggeredRule(t, r, "test_rule_replay_load_module")
				assert.Equal(t, uint64(model.ProcessCacheEntryFromUnknownLoader), e.ProcessContext.Source,
					"snapshot load_module events must be anchored on the synthetic unknown-loader PCE")
				gotEvent.Store(true)
			},
		}))
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		assert.Eventually(t, func() bool { return gotEvent.Load() }, 10*time.Second, 100*time.Millisecond, "didn't get the load_module event from snapshot")
	})
}

// readProcSnapshotCredentials independently reads the credential fields used by
// enrichEventFromProcfs. It is intentionally based on /proc directly rather
// than gopsutil so the test detects a parsing or index-mapping regression.
func readProcSnapshotCredentials(pid uint32) (model.Credentials, error) {
	var (
		credentials                                  model.Credentials
		foundUID, foundGID, foundCapEff, foundCapPrm bool
	)

	status, err := os.ReadFile(utils.StatusPath(pid))
	if err != nil {
		return credentials, err
	}

	for _, line := range strings.Split(string(status), "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		switch name {
		case "Uid":
			ids, err := parseProcCredentialIDs(value)
			if err != nil {
				return credentials, fmt.Errorf("parse Uid: %w", err)
			}
			credentials.UID = ids[0]
			credentials.EUID = ids[1]
			credentials.FSUID = ids[3]
			foundUID = true
		case "Gid":
			ids, err := parseProcCredentialIDs(value)
			if err != nil {
				return credentials, fmt.Errorf("parse Gid: %w", err)
			}
			credentials.GID = ids[0]
			credentials.EGID = ids[1]
			credentials.FSGID = ids[3]
			foundGID = true
		case "CapEff":
			credentials.CapEffective, err = strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				return credentials, fmt.Errorf("parse CapEff: %w", err)
			}
			foundCapEff = true
		case "CapPrm":
			credentials.CapPermitted, err = strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				return credentials, fmt.Errorf("parse CapPrm: %w", err)
			}
			foundCapPrm = true
		}
	}

	if !foundUID || !foundGID || !foundCapEff || !foundCapPrm {
		return credentials, fmt.Errorf("missing credentials in %s", utils.StatusPath(pid))
	}

	loginUID, err := os.ReadFile(utils.LoginUIDPath(pid))
	if os.IsNotExist(err) {
		credentials.AUID = sharedconsts.AuditUIDUnset
		return credentials, nil
	}
	if err != nil {
		return credentials, err
	}

	auid, err := strconv.ParseUint(strings.TrimSpace(string(loginUID)), 10, 32)
	if err != nil {
		return credentials, fmt.Errorf("parse loginuid: %w", err)
	}
	credentials.AUID = uint32(auid)
	return credentials, nil
}

func parseProcCredentialIDs(value string) ([4]uint32, error) {
	var ids [4]uint32
	fields := strings.Fields(value)
	if len(fields) != len(ids) {
		return ids, fmt.Errorf("expected %d IDs, got %d", len(ids), len(fields))
	}

	for i, field := range fields {
		id, err := strconv.ParseUint(field, 10, 32)
		if err != nil {
			return ids, err
		}
		ids[i] = uint32(id)
	}
	return ids, nil
}
