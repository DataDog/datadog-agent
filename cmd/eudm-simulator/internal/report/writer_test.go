// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func TestWriterReservesReportExclusively(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(&Report{Version: 2, Status: "running", RunID: "existing-run"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(path); !os.IsExist(err) {
		t.Fatal("second writer did not refuse the existing report")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("exclusive reservation changed existing report content")
	}
	assertPrivateReport(t, path)

	empty := filepath.Join(t.TempDir(), "reserved.json")
	if _, err := NewWriter(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(empty); !os.IsExist(err) {
		t.Fatal("second writer replaced an empty reservation")
	}
	assertPrivateReport(t, empty)
}

func TestWriterPeriodicSnapshotsRemainCompleteForReaders(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "report.json")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func(sequence int) *Report {
		return &Report{Version: 2, Status: "running", Seed: uint64(sequence), RunID: fmt.Sprintf("snapshot-%d", sequence), Errors: []string{strings.Repeat("complete-snapshot-", 1024)}}
	}
	if err := writer.Write(snapshot(0)); err != nil {
		t.Fatal(err)
	}
	read := func() error {
		file, err := filesystem.OpenShared(path)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		var report Report
		if err := json.Unmarshal(data, &report); err != nil {
			return err
		}
		if report.Version != 2 || report.RunID != fmt.Sprintf("snapshot-%d", report.Seed) || len(report.Errors) != 1 || report.Errors[0] != snapshot(0).Errors[0] {
			return errors.New("reader observed fields from different snapshots")
		}
		return nil
	}
	stop := make(chan struct{})
	errors := make(chan error, 4)
	var workers, ready sync.WaitGroup
	ready.Add(4)
	for range 4 {
		workers.Go(func() {
			first := true
			for {
				err := read()
				if first {
					ready.Done()
					first = false
				}
				if err != nil {
					errors <- err
					return
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		})
	}
	ready.Wait()
	for sequence := 1; sequence <= 30; sequence++ {
		if err := writer.Write(snapshot(sequence)); err != nil {
			close(stop)
			workers.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Fatalf("periodic report reader failed: %v", err)
	}
	if err := read(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	var final Report
	if err != nil || json.Unmarshal(data, &final) != nil || final.Seed != 30 {
		t.Fatal("final report is not the latest published snapshot")
	}
	assertPrivateReport(t, path)
	assertReportDirectory(t, directory, "report.json")
}

func TestWriterFailurePreservesSnapshotAndCleansTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "report.json")
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(&Report{Version: 2, RunID: "original"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []*Report{nil, {Start: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}} {
		if err := writer.Write(invalid); err == nil {
			t.Fatal("invalid report replaced the previous snapshot")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("encoding failure changed the previous snapshot")
		}
		assertReportDirectory(t, directory, "report.json")
	}

	// A directory at the destination forces rename to fail on every platform,
	// without permission assumptions that stop working under an elevated user.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(&Report{Version: 2, RunID: "replacement"}); err == nil {
		t.Fatal("report write replaced a directory")
	}
	assertReportDirectory(t, directory, "report.json")
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatal("rename failure modified the destination")
	}
}

func assertPrivateReport(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("report permissions are not private")
	}
}

func assertReportDirectory(t *testing.T, directory, expected string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != expected {
		t.Fatal("report writer left temporary files behind")
	}
}
