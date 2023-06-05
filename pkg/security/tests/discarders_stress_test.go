// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build stresstests

package tests

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	nbDiscardersRuns int
	testDuration     time.Duration
	maxTotalFiles    int
	eventsPerSec     int
	mountDir         bool
	mountParentDir   bool
	remountEvery     time.Duration
	maxDepth         int
	memTopFrom       string
	open             bool
)

type metric struct {
	vals []int64
	min  int64
	max  int64
	avg  int64
}

func computeMetrics(metrics map[string]*metric) {
	for _, metric := range metrics {
		metric.min = metric.vals[0]
		metric.max = metric.vals[0]
		total := metric.vals[0]
		for _, val := range metric.vals[1:] {
			if val > metric.max {
				metric.max = val
			} else if val < metric.min {
				metric.min = val
			}
			total += val
		}
		metric.avg = int64(total / int64(len(metric.vals)))
	}
}

func dumpMetrics(metrics map[string]*metric) {
	fmt.Printf("\nRESULT METRICS for %d runs of %v: \n", nbDiscardersRuns, testDuration)
	for id, metric := range metrics {
		if strings.Contains(id, "action.") {
			fmt.Printf("%s: %d (min: %d, max: %d)\n", id, metric.avg, metric.min, metric.max)
		}
	}
	fmt.Printf("---\n")
	for id, metric := range metrics {
		if strings.Contains(id, "datadog.") {
			fmt.Printf("%s: %d (min: %d, max: %d)\n", id, metric.avg, metric.min, metric.max)
		}
	}
	fmt.Printf("---\n")
	for id, metric := range metrics {
		if strings.Contains(id, "mem.") {
			fmt.Printf("%s: %d (min: %d, max: %d)\n", id, metric.avg, metric.min, metric.max)
		}
	}
}

func addMetricVal(ms map[string]*metric, key string, val int64) {
	m := ms[key]
	if m == nil {
		m = &metric{}
		ms[key] = m
	}
	m.vals = append(m.vals, val)
}

func addResultMetrics(res *EstimatedResult, metrics map[string]*metric) {
	addMetricVal(metrics, "action.file_creation", res.FileCreation)
	addMetricVal(metrics, "action.file_access", res.FileAccess)
	addMetricVal(metrics, "action.file_deletion", res.FileDeletion)
}

func addMemoryMetrics(t *testing.T, test *testModule, metrics map[string]*metric) error {
	runtime.GC()
	proMemFile, err := os.CreateTemp("/tmp", "stress-mem-")
	if err != nil {
		t.Error(err)
		return err
	}

	if err := pprof.WriteHeapProfile(proMemFile); err != nil {
		t.Error(err)
		return err
	}

	topDataMem, err := getTopData(proMemFile.Name(), memTopFrom, 50)
	if err != nil {
		t.Error(err)
		return err
	}

	fmt.Printf("\nMemory report:\n%s\n", string(topDataMem))
	return nil
}

func addModuleMetrics(test *testModule, ms map[string]*metric) {
	test.eventMonitor.SendStats()
	test.eventMonitor.SendStats()

	fmt.Printf("Metrics:\n")

	key := metrics.MetricDiscarderAdded + ":event_type:open"
	val := test.statsdClient.Get(key)
	key = metrics.MetricDiscarderAdded + ":event_type:unlink"
	val += test.statsdClient.Get(key)
	fmt.Printf("  %s:event_type:* %d\n", metrics.MetricDiscarderAdded, val)
	addMetricVal(ms, metrics.MetricDiscarderAdded, val)

	key = metrics.MetricEventDiscarded + ":event_type:open"
	val = test.statsdClient.Get(key)
	key = metrics.MetricEventDiscarded + ":event_type:unlink"
	val += test.statsdClient.Get(key)
	fmt.Printf("  %s:event_type:* %d\n", metrics.MetricEventDiscarded, val)
	addMetricVal(ms, metrics.MetricEventDiscarded, val)

	key = metrics.MetricPerfBufferEventsWrite + ":event_type:open"
	val = test.statsdClient.Get(key)
	key = metrics.MetricPerfBufferEventsWrite + ":event_type:unlink"
	val += test.statsdClient.Get(key)
	fmt.Printf("  %s:event_type:* %d\n", metrics.MetricPerfBufferEventsWrite, val)
	addMetricVal(ms, metrics.MetricPerfBufferEventsWrite, val)

	key = metrics.MetricPerfBufferEventsRead + ":event_type:open"
	val = test.statsdClient.Get(key)
	key = metrics.MetricPerfBufferEventsRead + ":event_type:unlink"
	val += test.statsdClient.Get(key)
	fmt.Printf("  %s:event_type:* %d\n", metrics.MetricPerfBufferEventsRead, val)
	addMetricVal(ms, metrics.MetricPerfBufferEventsRead, val)

	for _, key = range []string{
		metrics.MetricPerfBufferBytesWrite + ":map:events",
		metrics.MetricPerfBufferBytesRead + ":map:events",
		metrics.MetricDentryResolverHits + ":type:cache",
		metrics.MetricDentryResolverMiss + ":type:cache",
	} {
		val = test.statsdClient.Get(key)
		fmt.Printf("  %s: %d\n", key, val)
		addMetricVal(ms, key, val)
	}
}

// goal: measure the performance behavior of discarders on load
func runTestDiscarders(t *testing.T, metrics map[string]*metric) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "rule",
			Expression: fmt.Sprintf(`open.file.path =~ "{{.Root}}/files_generator_root/%s/no-approver-*"`, noDiscardersDirName),
		},
		{
			ID:         "rule2",
			Expression: fmt.Sprintf(`unlink.file.path =~ "{{.Root}}/files_generator_root/%s/no-approver-*"`, noDiscardersDirName),
		},
	}
	test, err := newTestModule(t, nil, rules, testOpts{enableActivityDump: false})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	rootPath, _, err := test.Path("files_generator_root")
	if err != nil {
		t.Fatal(err)
	}
	fileGen, err := NewFileGenerator(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rootPath)

	err = fileGen.PrepareFileGenerator(FileGeneratorConfig{
		id:             "parent_mount",
		TestDuration:   testDuration,
		Debug:          false,
		MaxTotalFiles:  maxTotalFiles,
		EventsPerSec:   eventsPerSec,
		MountDir:       mountDir,
		MountParentDir: mountParentDir,
		RemountEvery:   remountEvery,
		MaxDepth:       maxDepth,
		Open:           open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileGen.Start(); err != nil {
		t.Fatal(err)
	}
	res, err := fileGen.Wait()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Test result:\n")
	res.Print()
	addResultMetrics(res, metrics)
	res = nil

	addModuleMetrics(test, metrics)
	addMemoryMetrics(t, test, metrics)
}

// goal: measure the performance behavior of discarders on load
func TestDiscarders(t *testing.T) {
	metrics := make(map[string]*metric)

	for i := 0; i < nbDiscardersRuns; i++ {
		fmt.Printf("\nRUN: %d\n", i+1)
		runTestDiscarders(t, metrics)
	}
	computeMetrics(metrics)
	dumpMetrics(metrics)
}

func init() {
	flag.IntVar(&nbDiscardersRuns, "nb_discarders_runs", 5, "number of tests to run")
	flag.DurationVar(&testDuration, "test_duration", time.Second*60*5, "duration of the test")
	flag.IntVar(&maxTotalFiles, "max_total_files", 10000, "maximum number of files")
	flag.IntVar(&eventsPerSec, "events_per_sec", 2000, "max events per sec")
	flag.BoolVar(&mountDir, "mount_dir", true, "set to true to have a working directory tmpfs mounted")
	flag.BoolVar(&mountParentDir, "mount_parent_dir", false, "set to true to have a parent working directory tmpfs mounted")
	flag.DurationVar(&remountEvery, "remount_every", time.Second*60*3, "time between every mount points umount/remount")
	flag.IntVar(&maxDepth, "max_depth", 1, "directories max depth")
	flag.StringVar(&memTopFrom, "memory top from", "probe", "set to the package to filter for mem stats")
	flag.BoolVar(&open, "open", true, "true to enable randomly open events")
}
