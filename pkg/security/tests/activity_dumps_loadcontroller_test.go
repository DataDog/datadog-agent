// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activitydump "github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"

	"github.com/stretchr/testify/assert"
)

func TestActivityDumpsLoadControllerTimeout(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()
	assert.Equal(t, "11m0s", dump.Timeout)

	// trigg reducer (before t > timeout / 4)
	test.triggerLoadControlerReducer(dockerInstance, dump)

	// find the new dump, with timeout *= 3/4, or min timeout
	secondDump, err := test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "10m0s", secondDump.Timeout)
}

func TestActivityDumpsLoadControllerEventTypes(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	for activeEventTypes := activitydump.TracedEventTypesReductionOrder; ; activeEventTypes = activeEventTypes[1:] {
		// add all event types to the dump
		test.addAllEventTypesOnDump(dockerInstance, dump, syscallTester)
		time.Sleep(time.Second * 3)
		// trigg reducer
		test.triggerLoadControlerReducer(dockerInstance, dump)
		// find the new dump
		nextDump, err := test.findNextPartialDump(dockerInstance, dump)
		if err != nil {
			t.Fatal(err)
		}

		// extract all present event types present on the first dump
		presentEventTypes, err := test.extractAllDumpEventTypes(dump)
		if err != nil {
			t.Fatal(err)
		}
		if !isEventTypesStringSlicesEqual(activeEventTypes, presentEventTypes) {
			t.Fatalf("Dump's event types are different as expected (%v) vs (%v)", activeEventTypes, presentEventTypes)
		}
		if len(activeEventTypes) == 0 {
			break
		}
		dump = nextDump
	}
}

func TestActivityDumpsLoadControllerRateLimiter(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	// burst file creation
	testDir := filepath.Join(test.Root(), "ratelimiter")
	os.MkdirAll(testDir, os.ModePerm)
	test.dockerCreateFiles(dockerInstance, syscallTester, testDir, testActivityDumpRateLimiter*2)
	time.Sleep(time.Second * 3)
	// trigg reducer
	test.triggerLoadControlerReducer(dockerInstance, dump)
	// find the new dump, with ratelimiter *= 3/4
	secondDump, err := test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}

	// find the number of files creation that were added to the dump
	numberOfFiles, err := test.findNumberOfExistingDirectoryFiles(dump, testDir)
	if err != nil {
		t.Fatal(err)
	}
	if numberOfFiles < testActivityDumpRateLimiter/4 || numberOfFiles > testActivityDumpRateLimiter {
		t.Fatalf("number of files not expected (%d) with %d ratelimiter\n", numberOfFiles, testActivityDumpRateLimiter)
	}

	dump = secondDump
	// burst file creation
	test.dockerCreateFiles(dockerInstance, syscallTester, testDir, testActivityDumpRateLimiter*2)
	time.Sleep(time.Second * 3)
	// trigg reducer
	test.triggerLoadControlerReducer(dockerInstance, dump)
	// find the new dump, with ratelimiter *= 3/4
	_, err = test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}
	// find the number of files creation that were added to the dump
	numberOfFiles, err = test.findNumberOfExistingDirectoryFiles(dump, testDir)
	if err != nil {
		t.Fatal(err)
	}
	newRateLimiter := testActivityDumpRateLimiter / 4 * 3
	if numberOfFiles < newRateLimiter/4 || numberOfFiles > newRateLimiter {
		t.Fatalf("number of files not expected (%d) with %d ratelimiter\n", numberOfFiles, newRateLimiter)
	}
}

func isEventTypesStringSlicesEqual(slice1 []model.EventType, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}
firstLoop:
	for _, s1 := range slice1 {
		for _, s2 := range slice2 {
			if s1.String() == s2 {
				continue firstLoop
			}
		}
		return false
	}
	return true
}
