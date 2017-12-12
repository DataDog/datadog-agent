// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second
const defaultTTL = 23 * time.Hour

// A RegistryEntry represends an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	Timestamp   string
	Offset      int64
	LastUpdated time.Time
}

// An Auditor handles messages successfully submitted to the intake
type Auditor struct {
	inputChan     chan message.Message
	registry      map[string]*RegistryEntry
	registryMutex *sync.Mutex
	registryPath  string

	flushTicker   *time.Ticker
	flushPeriod   time.Duration
	cleanupTicker *time.Ticker
	cleanupPeriod time.Duration
	entryTTL      time.Duration
}

// New returns an initialized Auditor
func New(inputChan chan message.Message) *Auditor {
	return &Auditor{
		inputChan:     inputChan,
		registryPath:  filepath.Join(config.LogsAgent.GetString("run_path"), "registry.json"),
		registryMutex: &sync.Mutex{},

		flushPeriod:   defaultFlushPeriod,
		cleanupPeriod: defaultCleanupPeriod,
		entryTTL:      defaultTTL,
	}
}

// Start starts the Auditor
func (a *Auditor) Start() {
	a.registry = a.recoverRegistry(a.registryPath)
	a.cleanupRegistry(a.registry)
	go a.run()
	go a.flushRegistryPediodically()
	go a.cleanupRegistryPeriodically()
}

// flushRegistryPediodically periodically saves the registry in its current state
func (a *Auditor) flushRegistryPediodically() {
	a.flushTicker = time.NewTicker(a.flushPeriod)
	for {
		select {
		case <-a.flushTicker.C:
			err := a.flushRegistry(a.registry, a.registryPath)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// cleanupRegistryPeriodically periodically removes from the registry expired offsets
func (a *Auditor) cleanupRegistryPeriodically() {
	a.cleanupTicker = time.NewTicker(a.cleanupPeriod)
	for {
		select {
		case <-a.cleanupTicker.C:
			a.cleanupRegistry(a.registry)
		}
	}
}

// run lets the auditor update the registry
func (a *Auditor) run() {
	for msg := range a.inputChan {
		// An empty Identifier means that we don't want to track down the offset
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		if msg.GetOrigin().Identifier != "" {
			a.updateRegistry(msg.GetOrigin().Identifier, msg.GetOrigin().Offset, msg.GetOrigin().Timestamp)
		}
	}
}

// updateRegistry updates the offset of identifier in the auditor's registry
func (a *Auditor) updateRegistry(identifier string, offset int64, timestamp string) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	a.registry[identifier] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      offset,
		Timestamp:   timestamp,
	}
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *Auditor) recoverRegistry(path string) map[string]*RegistryEntry {
	mr, err := ioutil.ReadFile(path)
	if err != nil {
		log.Println(err)
		return make(map[string]*RegistryEntry)
	}
	r, err := a.unmarshalRegistry(mr)
	if err != nil {
		log.Println(err)
		return make(map[string]*RegistryEntry)
	}
	return r
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *Auditor) readOnlyRegistryCopy(registry map[string]*RegistryEntry) map[string]RegistryEntry {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	r := make(map[string]RegistryEntry)
	for path, entry := range registry {
		r[path] = *entry
	}
	return r
}

// flushRegistry writes on disk the registry at the given path
func (a *Auditor) flushRegistry(registry map[string]*RegistryEntry, path string) error {
	r := a.readOnlyRegistryCopy(registry)
	mr, err := a.marshalRegistry(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, mr, 0644)
}

// GetLastCommittedOffset returns the last committed offset for a given identifier
func (a *Auditor) GetLastCommittedOffset(identifier string) (int64, int) {
	r := a.readOnlyRegistryCopy(a.registry)
	entry, ok := r[identifier]
	if !ok {
		return 0, os.SEEK_END
	}
	return entry.Offset, os.SEEK_CUR
}

// GetLastCommittedTimestamp returns the last committed offset for a given identifier
func (a *Auditor) GetLastCommittedTimestamp(identifier string) string {
	r := a.readOnlyRegistryCopy(a.registry)
	entry, ok := r[identifier]
	if !ok {
		return ""
	}
	return entry.Timestamp
}

// cleanupRegistry removes expired entries from the registry
func (a *Auditor) cleanupRegistry(registry map[string]*RegistryEntry) {
	expireBefore := time.Now().UTC().Add(-a.entryTTL)
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	for path, entry := range registry {
		if entry.LastUpdated.Before(expireBefore) {
			delete(registry, path)
		}
	}
}

// JSONRegistry represents the registry that will be written on disk
type JSONRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// marshalRegistry marshals a registry
func (a *Auditor) marshalRegistry(registry map[string]RegistryEntry) ([]byte, error) {
	r := JSONRegistry{
		Version:  1,
		Registry: registry,
	}
	return json.Marshal(r)
}

// unmarshalRegistry unmarshals a registry
func (a *Auditor) unmarshalRegistry(b []byte) (map[string]*RegistryEntry, error) {
	var r JSONRegistry
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	if r.Version == 1 {
		for path, entry := range r.Registry {
			newEntry := entry
			registry[path] = &newEntry
		}
	} else if r.Version == 0 {
		return a.unmarshalRegistryV0(b)
	}
	return registry, nil
}

// Legacy Registry logic

type registryEntryV0 struct {
	Path      string
	Timestamp time.Time
	Offset    int64
}

type jsonRegistryV0 struct {
	Version  int
	Registry map[string]registryEntryV0
}

func (a *Auditor) unmarshalRegistryV0(b []byte) (map[string]*RegistryEntry, error) {
	var r jsonRegistryV0
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	registry := make(map[string]*RegistryEntry)
	for path, entry := range r.Registry {
		newEntry := RegistryEntry{}
		newEntry.Offset = entry.Offset
		newEntry.LastUpdated = entry.Timestamp
		newEntry.Timestamp = ""
		// from v0 to v1, we also prefixed path with file:
		newPath := fmt.Sprintf("file:%s", path)
		registry[newPath] = &newEntry
	}
	return registry, nil
}
