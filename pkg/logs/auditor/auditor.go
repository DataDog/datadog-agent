// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/status/health"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// DefaultRegistryFilename is the default registry filename
const DefaultRegistryFilename = "registry.json"

const defaultFlushPeriod = 1 * time.Second
const defaultCleanupPeriod = 300 * time.Second

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

// An Auditor handles messages successfully submitted to the intake
type Auditor interface {
	Registry
	Start()
	Stop()
	// Channel returns the channel to which successful payloads should be sent.
	Channel() chan *message.Payload
}

// A RegistryAuditor is storing the Auditor information using a registry.
type RegistryAuditor struct {
	health          *health.Handle
	chansMutex      sync.Mutex
	inputChan       chan *message.Payload
	registry        map[string]*RegistryEntry
	registryPath    string
	registryDirPath string
	registryTmpFile string
	registryMutex   sync.Mutex
	entryTTL        time.Duration
	done            chan struct{}
}

// New returns an initialized Auditor
func New(runPath string, filename string, ttl time.Duration, health *health.Handle) *RegistryAuditor {
	panic("not called")
}

// Start starts the Auditor
func (a *RegistryAuditor) Start() {
	panic("not called")
}

// Stop stops the Auditor
func (a *RegistryAuditor) Stop() {
	panic("not called")
}

func (a *RegistryAuditor) createChannels() {
	panic("not called")
}

func (a *RegistryAuditor) closeChannels() {
	panic("not called")
}

// Channel returns the channel to use to communicate with the auditor or nil
// if the auditor is currently stopped.
func (a *RegistryAuditor) Channel() chan *message.Payload {
	panic("not called")
}

// GetOffset returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *RegistryAuditor) GetOffset(identifier string) string {
	panic("not called")
}

// GetTailingMode returns the last committed offset for a given identifier,
// returns an empty string if it does not exist.
func (a *RegistryAuditor) GetTailingMode(identifier string) string {
	panic("not called")
}

// run keeps up to date the registry depending on different events
func (a *RegistryAuditor) run() {
	panic("not called")
}

// recoverRegistry rebuilds the registry from the state file found at path
func (a *RegistryAuditor) recoverRegistry() map[string]*RegistryEntry {
	panic("not called")
}

// cleanupRegistry removes expired entries from the registry
func (a *RegistryAuditor) cleanupRegistry() {
	panic("not called")
}

// updateRegistry updates the registry entry matching identifier with new the offset and timestamp
func (a *RegistryAuditor) updateRegistry(identifier string, offset string, tailingMode string, ingestionTimestamp int64) {
	panic("not called")
}

// readOnlyRegistryCopy returns a read only copy of the registry
func (a *RegistryAuditor) readOnlyRegistryCopy() map[string]RegistryEntry {
	panic("not called")
}

// flushRegistry writes on disk the registry at the given path
func (a *RegistryAuditor) flushRegistry() error {
	panic("not called")
}

// marshalRegistry marshals a registry
func (a *RegistryAuditor) marshalRegistry(registry map[string]RegistryEntry) ([]byte, error) {
	panic("not called")
}

// unmarshalRegistry unmarshals a registry
func (a *RegistryAuditor) unmarshalRegistry(b []byte) (map[string]*RegistryEntry, error) {
	panic("not called")
}
