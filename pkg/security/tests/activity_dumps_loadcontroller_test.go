// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activitydump "github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"

	"github.com/stretchr/testify/assert"
)

func TestActivityDumpsLoadControllerTimeout(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()

	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind", "imds"}
	opts := testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                time.Minute + 10*time.Second,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpLoadControllerPeriod:    testActivityDumpLoadControllerPeriod,
		activityDumpLoadControllerTimeout:   time.Minute,
		networkIngressEnabled:               true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
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
	assert.Equal(t, opts.activityDumpDuration.String(), dump.Timeout)

	// trigger reducer (before t > timeout / 4)
	test.triggerLoadControllerReducer(dockerInstance, dump)

	// find the new dump, with timeout *= 3/4, or min timeout
	secondDump, err := test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}

	timeout, err := time.ParseDuration(secondDump.Timeout)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, opts.activityDumpLoadControllerTimeout, timeout)
}

func TestActivityDumpsLoadControllerEventTypes(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()

	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind", "imds"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpLoadControllerPeriod:    testActivityDumpLoadControllerPeriod,
		networkIngressEnabled:               true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
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

	// setup IMDS interface
	cmd := dockerInstance.Command(goSyscallTester, []string{"-setup-imds-test"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ERROR: %v", err)
	}
	defer func() {
		cleanup := dockerInstance.Command(goSyscallTester, []string{"-cleanup-imds-test"}, []string{})
		_, err := cleanup.CombinedOutput()
		if err != nil {
			fmt.Printf("failed to cleanup IMDS test: %v", err)
		}
	}()

	for activeEventTypes := activitydump.TracedEventTypesReductionOrder; ; activeEventTypes = activeEventTypes[1:] {
		testName := ""
		for i, activeEventType := range activeEventTypes {
			if i > 0 {
				testName += "-"
			}
			testName += activeEventType.String()
		}
		if testName == "" {
			testName = "none"
		}
		t.Run(testName, func(t *testing.T) {
			// add all event types to the dump
			test.addAllEventTypesOnDump(dockerInstance, syscallTester, goSyscallTester)
			time.Sleep(time.Second * 3)
			// trigger reducer
			test.triggerLoadControllerReducer(dockerInstance, dump)
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
			activeTypes := make([]model.EventType, len(activeEventTypes))
			for i, eventType := range activeEventTypes {
				activeTypes[i] = eventType
			}
			if !slices.Contains(activeTypes, model.FileOpenEventType) {
				// add open to the list of expected event types because mmaped files being present in the dump
				activeTypes = append(activeTypes, model.FileOpenEventType)
			}
			if !isEventTypesStringSlicesEqual(activeTypes, presentEventTypes) {
				t.Fatalf("Dump's event types don't match: expected[%v] vs observed[%v]", activeEventTypes, presentEventTypes)
			}
			dump = nextDump
		})

		if len(activeEventTypes) == 0 {
			break
		}
	}
}

func TestActivityDumpsLoadControllerRateLimiter(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()

	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind", "imds"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpLoadControllerPeriod:    testActivityDumpLoadControllerPeriod,
		networkIngressEnabled:               true,
	}))
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
	test.triggerLoadControllerReducer(dockerInstance, dump)
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
	test.triggerLoadControllerReducer(dockerInstance, dump)
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
