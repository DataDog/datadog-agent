// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package auditorimpl implements the auditor component interface
package auditorimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	healthdef "github.com/DataDog/datadog-agent/comp/logs/health/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second

// latest version of the API used by the auditor to retrieve the registry from disk.
const registryAPIVersion = 2

// defaultRegistryFilename is the default registry filename
const defaultRegistryFilename = "registry.json"

// A RegistryEntry represents an entry in the registry where we keep track
// of current offsets
type RegistryEntry struct {
	LastUpdated        time.Time
	Offset             string
	TailingMode        string
	IngestionTimestamp int64
}

// JSONRegistry represents the registry that will be written on disk
type JSONRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// A registryAuditor is storing the Auditor information using a registry.
type registryAuditor struct {
	health             *health.Handle
	healthRegistrar    healthdef.Component
	chansMutex         sync.Mutex
	inputChan          chan *message.Payload
	registry           map[string]*RegistryEntry
	tailedSources      map[string]bool
	registryPath       string
	registryDirPath    string
	registryTmpFile    string
	registryMutex      sync.Mutex
	entryTTL           time.Duration
	done               chan struct{}
	messageChannelSize int
	registryWriter     auditor.RegistryWriter

	log log.Component
}

// Dependencies defines the dependencies of the auditor
type Dependencies struct {
	Log    log.Component
	Config config.Component
	Health healthdef.Component
}

// Provides contains the auditor component
type Provides struct {
	Comp auditor.Component
}

// NewAuditor is the public constructor for the auditor
func newAuditor(deps Dependencies) *registryAuditor {
	runPath := deps.Config.GetString("logs_config.run_path")
	filename := defaultRegistryFilename
	ttl := time.Duration(deps.Config.GetInt("logs_config.auditor_ttl")) * time.Hour
	messageChannelSize := deps.Config.GetInt("logs_config.message_channel_size")
	atomicRegistryWrite := deps.Config.GetBool("logs_config.atomic_registry_write")

	var registryWriter auditor.RegistryWriter
	if atomicRegistryWrite {
		registryWriter = NewAtomicRegistryWriter()
	} else {
		registryWriter = NewNonAtomicRegistryWriter()
	}

	registryAuditor := &registryAuditor{
		registryPath:       filepath.Join(runPath, filename),
		registryDirPath:    deps.Config.GetString("logs_config.run_path"),
		registryTmpFile:    filepath.Base(filename) + ".tmp",
		tailedSources:      make(map[string]bool),
		entryTTL:           ttl,
		messageChannelSize: messageChannelSize,
		log:                deps.Log,
		healthRegistrar:    deps.Health,
		registryWriter:     registryWriter,
	}

	return registryAuditor
}

// NewProvides creates a new auditor component
func NewProvides(deps Dependencies) Provides {
	auditorImpl := newAuditor(deps)
	return Provides{
		Comp: auditorImpl,
	}
}

// Start starts the Auditor
func (a *registryAuditor) Start() {
	a.health = a.healthRegistrar.RegisterLiveness("logs-agent")

	a.createChannels()
	a.registry = a.recoverRegistry()
	go a.run()
}

// Stop stops the Auditor
func (a *registryAuditor) Stop() {
	a.closeChannels()
	a.cleanupRegistry()
	if err := a.flushRegistry(); err != nil {
		a.log.Warn(err)
	}
}

func (a *registryAuditor) createChannels() {
	a.chansMutex.Lock()
	defer a.chansMutex.Unlock()
	a.inputChan = make(chan *message.Payload, a.messageChannelSize)
	a.done = make(chan struct{})
}

func (a *registryAuditor) closeChannels() {
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
func (a *registryAuditor) Channel() chan *message.Payload {
	a.chansMutex.Lock()
	defer a.chansMutex.Unlock()
	return a.inputChan
}

// GetOffset returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *registryAuditor) GetOffset(identifier string) string {
	entry, exists := a.readOnlyRegistryEntryCopy(identifier)
	if !exists {
		return ""
	}
	return entry.Offset
}

// GetTailingMode returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *registryAuditor) GetTailingMode(identifier string) string {
	entry, exists := a.readOnlyRegistryEntryCopy(identifier)
	if !exists {
		return ""
	}
	return entry.TailingMode
}

// KeepAlive modifies the last updated timestamp for a given identifier to signal that the identifier is still being tailed
// even if no new data is being received.
// This is used for entities that are not guaranteed to have a tailer assigned to them
func (a *registryAuditor) KeepAlive(identifier string) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	if _, ok := a.registry[identifier]; ok {
		a.registry[identifier].LastUpdated = time.Now().UTC()
	}
}

// SetTailed is used to signal the identifier's status for registry cleanup purposes
func (a *registryAuditor) SetTailed(identifier string, isTailed bool) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	if isTailed {
		a.tailedSources[identifier] = true
	} else {
		delete(a.tailedSources, identifier)
	}

	// entities that are no longer tailed should remain in the registry for the TTL period
	// in case they are tailed again.
	if _, ok := a.registry[identifier]; ok {
		a.registry[identifier].LastUpdated = time.Now().UTC()
	}
}

// run keeps up to date the registry on different events
func (a *registryAuditor) run() {
	cleanUpTicker := time.NewTicker(defaultCleanupPeriod)
	flushTicker := time.NewTicker(defaultFlushPeriod)

	defer func() {
		// Clean the context
		cleanUpTicker.Stop()
		flushTicker.Stop()
		a.done <- struct{}{}
	}()

	var fileError sync.Once
	for {
		select {
		case <-a.health.C:
		case payload, isOpen := <-a.inputChan:
			if !isOpen {
				// inputChan has been closed, no need to update the registry anymore
				return
			}
			// update the registry with the new entry
			for _, msg := range payload.MessageMetas {
				a.updateRegistry(msg.Origin.Identifier, msg.Origin.Offset, msg.Origin.LogSource.Config.TailingMode, msg.IngestionTimestamp)
			}
		case <-cleanUpTicker.C:
			// remove expired offsets from the registry
			a.cleanupRegistry()
		case <-flushTicker.C:
			// saves the current registry into disk
			err := a.flushRegistry()
			if err != nil {
				if os.IsPermission(err) || os.IsNotExist(err) {
					fileError.Do(func() {
						a.log.Warn(err)
					})
				} else {
					a.log.Warn(err)
				}
			}
		}
	}
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *registryAuditor) recoverRegistry() map[string]*RegistryEntry {
	mr, err := os.ReadFile(a.registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			a.log.Infof("Could not find state file at %q, will start with default offsets", a.registryPath)
		} else {
			a.log.Error(err)
		}
		return make(map[string]*RegistryEntry)
	}
	r, err := a.unmarshalRegistry(mr)
	if err != nil {
		a.log.Error(err)
		return make(map[string]*RegistryEntry)
	}

	return r
}

// cleanupRegistry removes expired entries from the registry
func (a *registryAuditor) cleanupRegistry() {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	expireBefore := time.Now().UTC().Add(-a.entryTTL)
	for path, entry := range a.registry {
		if entry.LastUpdated.Before(expireBefore) {
			if a.tailedSources[path] {
				a.log.Debugf("TTL for %s is expired but it is still tailed, keeping in registry.", path)
			} else {
				a.log.Debugf("TTL for %s is expired, removing from registry.", path)
				delete(a.registry, path)
			}
		}
	}
}

// updateRegistry updates the registry entry matching identifier with the new offset and timestamp
func (a *registryAuditor) updateRegistry(identifier string, offset string, tailingMode string, ingestionTimestamp int64) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	if identifier == "" {
		// An empty Identifier means that we don't want to track down the offset
		// This is useful for origins that don't have offsets (networks), or when we
		// specially want to avoid storing the offset
		return
	}

	// Don't update the registry with a value older than the current one
	// This can happen when dual shipping and 2 destinations are sending the same payload successfully
	if v, ok := a.registry[identifier]; ok {
		if v.IngestionTimestamp > ingestionTimestamp {
			return
		}
	}

	a.registry[identifier] = &RegistryEntry{
		LastUpdated:        time.Now().UTC(),
		Offset:             offset,
		TailingMode:        tailingMode,
		IngestionTimestamp: ingestionTimestamp,
	}
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *registryAuditor) readOnlyRegistryCopy() map[string]RegistryEntry {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	r := make(map[string]RegistryEntry)
	for path, entry := range a.registry {
		r[path] = *entry
	}
	return r
}

func (a *registryAuditor) readOnlyRegistryEntryCopy(identifier string) (RegistryEntry, bool) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	entry, exists := a.registry[identifier]
	if !exists {
		return RegistryEntry{}, false
	}
	return *entry, true
}

// flushRegistry writes on disk the registry at the given path
func (a *registryAuditor) flushRegistry() error {
	r := a.readOnlyRegistryCopy()
	mr, err := a.marshalRegistry(r)
	if err != nil {
		return err
	}
	return a.registryWriter.WriteRegistry(a.registryPath, a.registryDirPath, a.registryTmpFile, mr)
}

// marshalRegistry marshals a regsistry
func (a *registryAuditor) marshalRegistry(registry map[string]RegistryEntry) ([]byte, error) {
	r := JSONRegistry{
		Version:  registryAPIVersion,
		Registry: registry,
	}
	return json.Marshal(r)
}

// unmarshalRegistry unmarshals a registry
func (a *registryAuditor) unmarshalRegistry(b []byte) (map[string]*RegistryEntry, error) {
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
