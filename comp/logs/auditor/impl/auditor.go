// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package auditorimpl implements the auditor component interface
package auditorimpl

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	kubehealthdef "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// defaultFlushPeriod is the default interval between registry writes while
// the agent is running (logs_config.auditor_flush_interval). It also replaces
// a configured value that is not positive. Stop writes once more after
// acknowledgements queued at shutdown have been applied.
const defaultFlushPeriod = 5 * time.Second
const defaultCleanupPeriod = 300 * time.Second

// touchOnlyWritePeriod is the shortest interval between periodic registry
// writes when the only changes are LastUpdated refreshes from KeepAlive and
// SetTailed. LastUpdated is read only by cleanupRegistry, which runs on the
// same period against a TTL measured in hours.
const touchOnlyWritePeriod = defaultCleanupPeriod

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
	Fingerprint        types.Fingerprint
}

// JSONRegistry represents the registry that will be written on disk
type JSONRegistry struct {
	Version  int
	Registry map[string]RegistryEntry
}

// A registryAuditor is storing the Auditor information using a registry.
type registryAuditor struct {
	health              *health.Handle
	kubeHealthRegistrar kubehealthdef.Component
	chansMutex          sync.Mutex
	inputChan           chan *message.Payload
	flushRequestChan    chan chan struct{}
	registry            map[string]*RegistryEntry
	tailedSources       map[string]bool
	registryPath        string
	registryDirPath     string
	registryTmpFile     string
	registryMutex       sync.Mutex
	entryTTL            time.Duration
	flushPeriod         time.Duration
	done                chan struct{}
	messageChannelSize  int
	registryWriter      auditor.RegistryWriter

	// changeGeneration counts registry changes other than LastUpdated
	// refreshes: entries added, updated or removed. touchGeneration counts
	// LastUpdated refreshes from KeepAlive and SetTailed. Both are guarded by
	// registryMutex.
	changeGeneration uint64
	touchGeneration  uint64

	// flushMutex serializes registry writes. It guards the generations and
	// the time of the last write to disk.
	flushMutex              sync.Mutex
	writtenChangeGeneration uint64
	writtenTouchGeneration  uint64
	lastWrite               time.Time

	log log.Component
}

// Dependencies defines the dependencies of the auditor
type Dependencies struct {
	Log        log.Component
	Config     config.Component
	KubeHealth kubehealthdef.Component
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
	flushPeriod := flushPeriodFromConfig(deps)

	var registryWriter auditor.RegistryWriter
	if atomicRegistryWrite {
		registryWriter = NewAtomicRegistryWriter()
	} else {
		registryWriter = NewNonAtomicRegistryWriter()
	}

	registryAuditor := &registryAuditor{
		registryPath:        filepath.Join(runPath, filename),
		registryDirPath:     deps.Config.GetString("logs_config.run_path"),
		registryTmpFile:     filepath.Base(filename) + ".tmp",
		tailedSources:       make(map[string]bool),
		entryTTL:            ttl,
		flushPeriod:         flushPeriod,
		messageChannelSize:  messageChannelSize,
		log:                 deps.Log,
		kubeHealthRegistrar: deps.KubeHealth,
		registryWriter:      registryWriter,
	}

	return registryAuditor
}

// flushPeriodFromConfig reads logs_config.auditor_flush_interval, in seconds.
// A value that is not positive, or that rounds to less than a nanosecond,
// falls back to defaultFlushPeriod because time.NewTicker rejects it.
func flushPeriodFromConfig(deps Dependencies) time.Duration {
	seconds := deps.Config.GetFloat64("logs_config.auditor_flush_interval")
	period := time.Duration(seconds * float64(time.Second))
	if period <= 0 {
		deps.Log.Warnf("logs_config.auditor_flush_interval must be greater than 0, got %v; using the default of %v", seconds, defaultFlushPeriod)
		return defaultFlushPeriod
	}
	return period
}

// NewComponent creates a new auditor component
func NewComponent(deps Dependencies) Provides {
	auditorImpl := newAuditor(deps)
	return Provides{
		Comp: auditorImpl,
	}
}

// Start starts the Auditor
func (a *registryAuditor) Start() {
	a.health = a.kubeHealthRegistrar.RegisterLiveness("logs-agent")

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

// Flush drains all pending payloads from the input channel, updates the
// in-memory registry, then writes it to disk. It blocks until complete.
// When the auditor is stopped (run loop not active), it falls back to
// writing the current in-memory registry directly.
//
// Must not be called concurrently with Stop.
func (a *registryAuditor) Flush() {
	a.chansMutex.Lock()
	reqChan := a.flushRequestChan
	a.chansMutex.Unlock()

	if reqChan == nil {
		if err := a.flushRegistry(); err != nil {
			a.log.Warnf("Failed to flush auditor registry: %v", err)
		}
		return
	}

	done := make(chan struct{})
	reqChan <- done
	<-done
}

func (a *registryAuditor) createChannels() {
	a.chansMutex.Lock()
	defer a.chansMutex.Unlock()
	a.inputChan = make(chan *message.Payload, a.messageChannelSize)
	a.flushRequestChan = make(chan chan struct{})
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
	a.flushRequestChan = nil
}

// GetFingerprint returns the fingerprint for a given identifier,
// returns nil if it doesn't exist.
func (a *registryAuditor) GetFingerprint(identifier string) *types.Fingerprint {
	entry, exists := a.readOnlyRegistryEntryCopy(identifier)
	if !exists {
		return nil
	}
	return &entry.Fingerprint
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
	if entry, ok := a.registry[identifier]; ok {
		entry.LastUpdated = time.Now().UTC()
		a.touchGeneration++
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
	if entry, ok := a.registry[identifier]; ok {
		entry.LastUpdated = time.Now().UTC()
		a.touchGeneration++
	}
}

// SetOffset allows direct setting of an offset for an identifier, marking it as tailed
func (a *registryAuditor) SetOffset(identifier string, offset string) {
	a.registryMutex.Lock()
	defer a.registryMutex.Unlock()
	a.changeGeneration++
	if entry, exists := a.registry[identifier]; exists {
		entry.Offset = offset
		entry.LastUpdated = time.Now().UTC()
	} else {
		// Create a new entry if it doesn't exist
		a.registry[identifier] = &RegistryEntry{
			LastUpdated: time.Now().UTC(),
			Offset:      offset,
			TailingMode: "", // Default to empty, can be set later
		}
	}
}

// run keeps up to date the registry on different events
func (a *registryAuditor) run() {
	cleanUpTicker := time.NewTicker(defaultCleanupPeriod)
	flushTicker := time.NewTicker(a.flushPeriod)

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
				// A closed channel yields every buffered payload before this
				// receive, so the in-memory registry already includes the acks
				// that were queued when Stop closed the input. Stop writes that
				// registry after this loop exits.
				return
			}
			// update the registry with the new entry
			for _, msg := range payload.MessageMetas {
				var fingerprint types.Fingerprint
				if msg.Origin.Fingerprint != nil {
					fingerprint = *msg.Origin.Fingerprint
				}
				a.updateRegistry(msg.Origin.Identifier, msg.Origin.Offset, msg.Origin.LogSource.Config.TailingMode, msg.IngestionTimestamp, fingerprint)
			}
		case <-cleanUpTicker.C:
			// remove expired offsets from the registry
			a.cleanupRegistry()
		case <-flushTicker.C:
			// saves the current registry into disk when it has changed
			err := a.flushRegistryIfChanged(time.Now())
			if err != nil {
				if os.IsPermission(err) || os.IsNotExist(err) {
					fileError.Do(func() {
						a.log.Warn(err)
					})
				} else {
					a.log.Warn(err)
				}
			}
		case responseChan := <-a.flushRequestChan:
			n := len(a.inputChan)
			for i := 0; i < n; i++ {
				select {
				case payload := <-a.inputChan:
					for _, msg := range payload.MessageMetas {
						var fingerprint types.Fingerprint
						if msg.Origin.Fingerprint != nil {
							fingerprint = *msg.Origin.Fingerprint
						}
						a.updateRegistry(msg.Origin.Identifier, msg.Origin.Offset, msg.Origin.LogSource.Config.TailingMode, msg.IngestionTimestamp, fingerprint)
					}
				default:
				}
			}
			if err := a.flushRegistry(); err != nil {
				a.log.Warnf("Flush: failed to flush registry: %v", err)
			}
			close(responseChan)
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
				a.changeGeneration++
			}
		}
	}
}

// updateRegistry updates the registry with a new entry
func (a *registryAuditor) updateRegistry(identifier string, offset string, tailingMode string, ingestionTimestamp int64, fingerprint types.Fingerprint) {
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
		Fingerprint:        fingerprint,
	}
	a.changeGeneration++
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

// flushRegistry writes the registry to disk, whether or not it has changed
// since the last write.
func (a *registryAuditor) flushRegistry() error {
	a.flushMutex.Lock()
	defer a.flushMutex.Unlock()
	return a.writeRegistry(time.Now())
}

// flushRegistryIfChanged writes the registry to disk when an entry has been
// added, updated or removed since the last write, or when entries have only
// had LastUpdated refreshed and the last write is at least
// touchOnlyWritePeriod old.
func (a *registryAuditor) flushRegistryIfChanged(now time.Time) error {
	a.flushMutex.Lock()
	defer a.flushMutex.Unlock()

	a.registryMutex.Lock()
	changed := a.changeGeneration != a.writtenChangeGeneration
	touched := a.touchGeneration != a.writtenTouchGeneration
	a.registryMutex.Unlock()

	if !changed && !(touched && now.Sub(a.lastWrite) >= touchOnlyWritePeriod) {
		return nil
	}
	return a.writeRegistry(now)
}

// writeRegistry streams the registry to disk with encodeRegistry. The caller
// holds flushMutex.
func (a *registryAuditor) writeRegistry(now time.Time) error {
	var changeGeneration, touchGeneration uint64
	err := a.registryWriter.WriteRegistry(a.registryPath, a.registryDirPath, a.registryTmpFile, func(w io.Writer) error {
		var err error
		changeGeneration, touchGeneration, err = a.encodeRegistry(w)
		return err
	})
	if err != nil {
		return err
	}
	a.writtenChangeGeneration, a.writtenTouchGeneration = changeGeneration, touchGeneration
	a.lastWrite = now
	return nil
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
		return nil, errors.New("registry retrieved from disk must have a version number")
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
		return nil, errors.New("invalid registry version number")
	}
}
