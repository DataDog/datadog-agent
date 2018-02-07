// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second
const defaultTTL = 23 * time.Hour

// A RegistryEntry represents an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	Timestamp   string
	Offset      int64
	LastUpdated time.Time
}

// An Auditor handles messages successfully submitted to the intake
type Auditor struct {
	inputChan    chan message.Message
	registry     map[string]*RegistryEntry
	registryPath string

	entryTTL time.Duration

	done chan struct{}
}

// New returns an initialized Auditor
func New(inputChan chan message.Message, runPath string) *Auditor {
	return &Auditor{
		inputChan:    inputChan,
		registryPath: filepath.Join(runPath, "registry.json"),
		entryTTL:     defaultTTL,
		done:         make(chan struct{}),
	}
}

// Start starts the Auditor
func (a *Auditor) Start() {
	a.registry = a.recoverRegistry(a.registryPath)
	a.cleanupRegistry(a.registry)
	go a.run()
}

// Stop stops the Auditor
func (a *Auditor) Stop() {
	close(a.inputChan)
	<-a.done
	a.cleanupRegistry(a.registry)
	err := a.flushRegistry(a.registry, a.registryPath)
	if err != nil {
		log.Warn(err)
	}
}

// run keeps up to date the registry depending on different events
func (a *Auditor) run() {
	cleanUpTicker := time.NewTicker(defaultCleanupPeriod)
	flushTicker := time.NewTicker(defaultFlushPeriod)
	defer func() {
		// clean the context
		cleanUpTicker.Stop()
		flushTicker.Stop()
		a.done <- struct{}{}
	}()

	for {
		select {
		case msg, isOpen := <-a.inputChan:
			if !isOpen {
				// inputChan has been closed, no need to update the registry anymore
				return
			}
			// update the registry with new entry
			a.updateRegistry(msg.GetOrigin().Identifier, msg.GetOrigin().Offset, msg.GetOrigin().Timestamp)
		case <-cleanUpTicker.C:
			// remove expired offsets from registry
			a.cleanupRegistry(a.registry)
		case <-flushTicker.C:
			// saves current registry into disk
			err := a.flushRegistry(a.registry, a.registryPath)
			if err != nil {
				log.Warn(err)
			}
		}
	}
}

// updateRegistry updates the registry entry matching identifier with new the offset and timestamp
func (a *Auditor) updateRegistry(identifier string, offset int64, timestamp string) {
	if identifier == "" {
		// An empty Identifier means that we don't want to track down the offset
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		return
	}
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
		log.Error(err)
		return make(map[string]*RegistryEntry)
	}
	r, err := a.unmarshalRegistry(mr)
	if err != nil {
		log.Error(err)
		return make(map[string]*RegistryEntry)
	}
	return r
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *Auditor) readOnlyRegistryCopy(registry map[string]*RegistryEntry) map[string]RegistryEntry {
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
