// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package auditor

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second
const defaultTTL = 23 * time.Hour

// latest version of the API used by the auditor to retrieve the registry from disk.
const registryAPIVersion = 2

// Registry holds a list of offsets.
type Registry interface {
	GetOffset(identifier string) string
	GetTailingMode(identifier string) string
}

// A RegistryEntry represents an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	LastUpdated time.Time
	Offset      string
	TailingMode string
}

// JSONRegistry represents the registry that will be written on disk
type JSONRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// An Auditor handles messages successfully submitted to the intake
type Auditor struct {
	health        *health.Handle
	chansMutex    sync.Mutex
	inputChan     chan *message.Message
	registry      map[string]*RegistryEntry
	registryPath  string
	registryMutex sync.Mutex
	entryTTL      time.Duration
	done          chan struct{}
}

// New returns an initialized Auditor
func New(runPath string, health *health.Handle) *Auditor {
	return &Auditor{
		health:       health,
		registryPath: filepath.Join(runPath, "registry.json"),
		entryTTL:     defaultTTL,
	}
}

// Start starts the Auditor
func (a *Auditor) Start() {
	a.createChannels()
	a.registry = a.recoverRegistry()
	a.cleanupRegistry()
	go a.run()
}

// Stop stops the Auditor
func (a *Auditor) Stop() {
	a.closeChannels()
	a.cleanupRegistry()
	if err := a.flushRegistry(); err != nil {
		log.Warn(err)
	}
}

func (a *Auditor) createChannels() {
	a.chansMutex.Lock()
	defer a.chansMutex.Unlock()
	a.inputChan = make(chan *message.Message, config.ChanSize)
	a.done = make(chan struct{})
}

func (a *Auditor) closeChannels() {
	a.chansMutex.Lock()
	defer a.chansMutex.Unlock()
	if a.inputChan != nil {
		close(a.inputChan)
	}

	if a.done != nil {
		<-a.done
		a.done = nil
	}
	a.inputChan = nil
}

// Channel returns the channel to use to communicate with the auditor or nil
// if the auditor is currently stopped.
func (a *Auditor) Channel() chan *message.Message {
	return a.inputChan
}

// GetOffset returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *Auditor) GetOffset(identifier string) string {
	r := a.readOnlyRegistryCopy()
	entry, exists := r[identifier]
	if !exists {
		return ""
	}
	return entry.Offset
}

// GetTailingMode returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *Auditor) GetTailingMode(identifier string) string {
	r := a.readOnlyRegistryCopy()
	entry, exists := r[identifier]
	if !exists {
		return ""
	}
	return entry.TailingMode
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

	var fileError sync.Once
	for {
		select {
		case <-a.health.C:
		case msg, isOpen := <-a.inputChan:
			if !isOpen {
				// inputChan has been closed, no need to update the registry anymore
				return
			}
			// update the registry with new entry
			a.updateRegistry(msg.Origin.Identifier, msg.Origin.Offset, msg.Origin.LogSource.Config.TailingMode)
		case <-cleanUpTicker.C:
			// remove expired offsets from registry
			a.cleanupRegistry()
		case <-flushTicker.C:
			// saves current registry into disk
			err := a.flushRegistry()
			if err != nil {
				if os.IsPermission(err) || os.IsNotExist(err) {
					fileError.Do(func() {
						log.Warn(err)
					})
				} else {
					log.Warn(err)
				}
			}
		}
	}
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *Auditor) recoverRegistry() map[string]*RegistryEntry {
	mr, err := ioutil.ReadFile(a.registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debugf("Could not find state file at %q, will start with default offsets", a.registryPath)
		} else {
			log.Error(err)
		}
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
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	expireBefore := time.Now().UTC().Add(-a.entryTTL)
	for path, entry := range a.registry {
		if entry.LastUpdated.Before(expireBefore) {
			delete(a.registry, path)
		}
	}
}

// updateRegistry updates the registry entry matching identifier with new the offset and timestamp
func (a *Auditor) updateRegistry(identifier string, offset string, tailingMode string) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	if identifier == "" {
		// An empty Identifier means that we don't want to track down the offset
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		return
	}
	a.registry[identifier] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      offset,
		TailingMode: tailingMode,
	}
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *Auditor) readOnlyRegistryCopy() map[string]RegistryEntry {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
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
