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

	"github.com/DataDog/datadog-agent/pkg/status/health"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// DefaultRegistryFilename is the default registry filename
const DefaultRegistryFilename = "registry.json"

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second

// latest version of the API used by the auditor to retrieve the registry from disk.
const registryAPIVersion = 2

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
	chansMutex         sync.Mutex
	inputChan          chan *message.Payload
	registry           map[string]*RegistryEntry
	registryPath       string
	registryDirPath    string
	registryTmpFile    string
	registryMutex      sync.Mutex
	entryTTL           time.Duration
	done               chan struct{}
	messageChannelSize int

	log log.Component
}

// Dependencies defines the dependencies of the auditor
type Dependencies struct {
	Log    log.Component
	Config config.Component
}

// Provides contains the auditor component
type Provides struct {
	Comp auditor.Component
}

// newAuditor is the public constructor for the auditor
func newAuditor(deps Dependencies) *registryAuditor {
	runPath := deps.Config.GetString("logs_config.run_path")
	// filename := deps.Config.GetString("logs_config.registry_filename")
	filename := DefaultRegistryFilename
	ttl := time.Duration(deps.Config.GetInt("logs_config.auditor_ttl")) * time.Hour
	messageChannelSize := deps.Config.GetInt("logs_config.message_channel_size")

	registryAuditor := &registryAuditor{
		registryPath:       filepath.Join(runPath, filename),
		registryDirPath:    deps.Config.GetString("logs_config.run_path"),
		registryTmpFile:    filepath.Base(filename) + ".tmp",
		entryTTL:           ttl,
		messageChannelSize: messageChannelSize,
		log:                deps.Log,
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
	health := health.RegisterLiveness("logs-agent")
	a.health = health

	a.createChannels()
	a.registry = a.recoverRegistry()
	a.cleanupRegistry()
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
			a.log.Debugf("TTL for %s is expired, removing from registry.", path)
			delete(a.registry, path)
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
	f, err := os.CreateTemp(a.registryDirPath, a.registryTmpFile)
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = f.Write(mr); err != nil {
		return err
	}
	if err = f.Chmod(0644); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	err = os.Rename(tmpName, a.registryPath)
	return err
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
