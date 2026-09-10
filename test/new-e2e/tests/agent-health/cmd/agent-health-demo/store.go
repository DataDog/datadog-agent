// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// envRecord is the persisted metadata for a provisioned demo environment. It
// lets run/list/delete operate without re-running Pulumi. It mirrors the record
// the former `dda lab demo` plugin stored, kept minimal for this CLI.
type envRecord struct {
	// Name is the un-prefixed stack name (the value passed to the stack manager).
	Name string `json:"name"`
	// Scenario is the demo scenario the env was created for (may be empty).
	Scenario string `json:"scenario,omitempty"`
	// HostIP is the provisioned host address, read from stack outputs.
	HostIP string `json:"host_ip,omitempty"`
	// SSHUser is the login user for the host.
	SSHUser string `json:"ssh_user"`
	// Site is the Datadog site the agent reports to.
	Site string `json:"site,omitempty"`
	// CreatedAt is the RFC3339 creation timestamp.
	CreatedAt string `json:"created_at"`
}

func storeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "agent-health-demo", "environments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func recordPath(name string) (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

func saveRecord(rec envRecord) error {
	if rec.CreatedAt == "" {
		rec.CreatedAt = time.Now().Format(time.RFC3339)
	}
	path, err := recordPath(rec.Name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadRecord(name string) (envRecord, error) {
	var rec envRecord
	path, err := recordPath(name)
	if err != nil {
		return rec, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	return rec, json.Unmarshal(data, &rec)
}

func deleteRecord(name string) error {
	path, err := recordPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// loadAllRecords returns every stored record, most recent first.
func loadAllRecords() ([]envRecord, error) {
	dir, err := storeDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var recs []envRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rec envRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].CreatedAt > recs[j].CreatedAt })
	return recs, nil
}

// latestRecord returns the most recent record, optionally filtered by scenario.
func latestRecord(scenario string) (envRecord, error) {
	recs, err := loadAllRecords()
	if err != nil {
		return envRecord{}, err
	}
	for _, r := range recs {
		if scenario == "" || r.Scenario == scenario {
			return r, nil
		}
	}
	return envRecord{}, fmt.Errorf("no demo environments found; run `create` first")
}
