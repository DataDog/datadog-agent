// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second
const defaultTTL = 23 * time.Hour

// latest version of the API used by the auditor to retrieve the registry from disk.
const registryAPIVersion = 2

// A RegistryEntry represents an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	LastUpdated time.Time
	Offset      string
}

// JSONRegistry represents the registry that will be written on disk
type JSONRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// An Auditor handles messages successfully submitted to the intake
type Auditor struct {
	inputChan    chan message.Message
	registry     map[string]*RegistryEntry
	registryPath string
	mu           sync.Mutex
	entryTTL     time.Duration
	done         chan struct{}
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
	a.registry = a.recoverRegistry()
	a.cleanupRegistry()
	go a.run()
}

// Stop stops the Auditor
func (a *Auditor) Stop() {
	close(a.inputChan)
	<-a.done
	a.cleanupRegistry()
	err := a.flushRegistry()
	if err != nil {
		log.Warn(err)
	}
}

// GetLastCommittedOffset returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *Auditor) GetLastCommittedOffset(identifier string) string {
	r := a.readOnlyRegistryCopy()
	entry, exists := r[identifier]
	if !exists {
		return ""
	}
	return entry.Offset
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
			a.updateRegistry(msg.GetOrigin().Identifier, msg.GetOrigin().Offset)
		case <-cleanUpTicker.C:
			// remove expired offsets from registry
			a.cleanupRegistry()
		case <-flushTicker.C:
			// saves current registry into disk
			err := a.flushRegistry()
			if err != nil {
				log.Warn(err)
			}
		}
	}
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *Auditor) recoverRegistry() map[string]*RegistryEntry {
	mr, err := ioutil.ReadFile(a.registryPath)
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

// cleanupRegistry removes expired entries from the registry
func (a *Auditor) cleanupRegistry() {
	a.mu.Lock()
	defer a.mu.Unlock()
	expireBefore := time.Now().UTC().Add(-a.entryTTL)
	for path, entry := range a.registry {
		if entry.LastUpdated.Before(expireBefore) {
			delete(a.registry, path)
		}
	}
}

// updateRegistry updates the registry entry matching identifier with new the offset and timestamp
func (a *Auditor) updateRegistry(identifier string, offset string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if identifier == "" {
		// An empty Identifier means that we don't want to track down the offset
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		return
	}
	a.registry[identifier] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      offset,
	}
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *Auditor) readOnlyRegistryCopy() map[string]RegistryEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := make(map[string]RegistryEntry)
	for path, entry := range a.registry {
		r[path] = *entry
	}
	return r
}

// flushRegistry writes on disk the registry at the given path
func (a *Auditor) flushRegistry() error {
	r := a.readOnlyRegistryCopy()
	mr, err := a.marshalRegistry(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(a.registryPath, mr, 0644)
}

// marshalRegistry marshals a registry
func (a *Auditor) marshalRegistry(registry map[string]RegistryEntry) ([]byte, error) {
	r := JSONRegistry{
		Version:  registryAPIVersion,
		Registry: registry,
	}
	return json.Marshal(r)
}

// unmarshalRegistry unmarshals a registry
func (a *Auditor) unmarshalRegistry(b []byte) (map[string]*RegistryEntry, error) {
	var r map[string]interface{}
	err := json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	version, exists := r["Version"].(float64)
	if !exists {
		return nil, fmt.Errorf("registry retrieved from disk must have a version number")
	}
	// ensure backward compatibility
	switch int(version) {
	case 2:
		return unmarshalRegistryV2(b)
	case 1:
		return unmarshalRegistryV1(b)
	case 0:
		return unmarshalRegistryV0(b)
	default:
		return nil, fmt.Errorf("invalid registry version number")
	}
}
