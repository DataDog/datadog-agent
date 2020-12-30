// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func benchmarkOpen(b *testing.B, rule *rules.RuleDefinition, pathname string, size int) {
	var rules []*rules.RuleDefinition
	if rule != nil {
		rules = append(rules, rule)
	}

	test, err := newTestModule(nil, rules, testOpts{wantProbeEvents: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFolder, _, err := test.Path(path.Dir(pathname))
	if err != nil {
		b.Fatal(err)
	}

	os.MkdirAll(testFolder, os.ModePerm)

	testFile, _, err := test.Path(pathname)
	if err != nil {
		b.Fatal(err)
	}

	eventsStats := test.probe.GetEventsStats()
	eventsStats.GetAndResetLost()

	b.ResetTimer()

	count := 0
	go func() {
		for event := range test.probeHandler.events {
			if probe.EventType(event.Type) == probe.FileOpenEventType {
				if flags := event.Open.Flags; flags&syscall.O_CREAT != 0 {
					filename, err := event.GetFieldValue("open.filename")
					if err == nil && filename.(string) == testFile {
						count++
					}
				}
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		f, err := os.Create(testFile)
		if err != nil {
			b.Fatal(err)
		}

		if size > 0 {
			data := make([]byte, size, size)
			if n, err := f.Write(data); err != nil || n != 1024 {
				b.Fatal(err)
			}
		}

		if err := f.Close(); err != nil {
			b.Fatal(err)
		}
	}

	time.Sleep(5 * time.Second)

	lost := eventsStats.GetLost()

	b.ReportMetric(float64(lost), "lost")
	b.ReportMetric(float64(count), "events")
	b.ReportMetric(100*float64(count)/float64(b.N), "%seen")
	b.ReportMetric(100*float64(lost)/float64(b.N), "%lost")
}

// goal: measure host abality to handle open syscall without any kprobe, act as a reference
// this benchmark generate syscall but without having kprobe installed

func BenchmarkE2EOpenNoKprobe(b *testing.B) {
	benchmarkOpen(b, nil, "folder1/folder2/folder1/folder2/test", 0)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func BenchmarkE2EOpenEvent(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
	}

	benchmarkOpen(b, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact on the kprobe only
// this benchmark generate syscall but without having event generated
func BenchmarkE2EOpenNoEvent(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	benchmarkOpen(b, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func BenchmarkE2EOpenWrite1KEvent(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
	}

	benchmarkOpen(b, rule, "folder1/folder2/test", 1024)
}

// goal: measure host abality to handle open syscall without any kprobe, act as a reference
// this benchmark generate syscall but without having kprobe installed

func BenchmarkE2EOpenWrite1KNoKprobe(b *testing.B) {
	benchmarkOpen(b, nil, "folder1/folder2/test", 1024)
}

// goal: measure the impact on the kprobe only
// this benchmark generate syscall but without having event generated
func BenchmarkE2EOpenWrite1KNoEvent(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	benchmarkOpen(b, rule, "folder1/folder2/test", 1024)
}
