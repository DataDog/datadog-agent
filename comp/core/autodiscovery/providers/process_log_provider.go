// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

const processLogProviderName = "process_log"

// serviceLogRef tracks reference count and config for a service+log combination
type serviceLogRef struct {
	refCount int
	config   integration.Config
}

// ProcessLogConfigProvider implements the ConfigProvider interface for processes with log files.
type ProcessLogConfigProvider struct {
	workloadmetaStore    workloadmeta.Component
	serviceLogRefs       map[string]*serviceLogRef // map[serviceLogKey]*serviceLogRef
	pidToServiceIDs      map[int32][]string        // map[pid][]serviceLogKey
	unreadableFilesCache *simplelru.LRU[string, struct{}]
	mu                   sync.RWMutex
}

var _ types.ConfigProvider = &ProcessLogConfigProvider{}
var _ types.StreamingConfigProvider = &ProcessLogConfigProvider{}

// NewProcessLogConfigProvider returns a new ConfigProvider subscribed to process events
func NewProcessLogConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ *telemetry.Store) (types.ConfigProvider, error) {
	cache, err := simplelru.NewLRU[string, struct{}](128, nil)
	if err != nil {
		return nil, err
	}
	return &ProcessLogConfigProvider{
		workloadmetaStore:    wmeta,
		serviceLogRefs:       make(map[string]*serviceLogRef),
		pidToServiceIDs:      make(map[int32][]string),
		unreadableFilesCache: cache,
	}, nil
}

// String returns a string representation of the ProcessLogConfigProvider
func (p *ProcessLogConfigProvider) String() string {
	return processLogProviderName
}

// Stream starts listening to workloadmeta to generate configs as they come
func (p *ProcessLogConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindProcess).
		Build()
	inCh := p.workloadmetaStore.Subscribe("process-log-provider", workloadmeta.ConfigProviderPriority, filter)

	go func() {
		for {
			select {
			case <-ctx.Done():
				p.workloadmetaStore.Unsubscribe(inCh)
				return
			case evBundle, ok := <-inCh:
				if !ok {
					return
				}
				changes := p.processEvents(evBundle)
				evBundle.Acknowledge()
				outCh <- changes
			}
		}
	}()

	return outCh
}

// generateServiceLogKey creates a unique key for service name + sanitized log path
func (p *ProcessLogConfigProvider) generateServiceLogKey(serviceName, logPath string) string {
	// Sanitize log path by replacing slashes with underscores
	sanitizedPath := strings.ReplaceAll(logPath, "/", "_")
	return fmt.Sprintf("%s:%s", serviceName, sanitizedPath)
}

func (p *ProcessLogConfigProvider) processEventsNoVerifyReadable(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	return p.processEventsInner(evBundle, false)
}

func (p *ProcessLogConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	return p.processEventsInner(evBundle, true)
}

func isFileReadable(logPath string) bool {
	file, err := os.Open(logPath)
	if err != nil {
		log.Infof("Discovered log file %s could not be opened: %v", logPath, err)
		return false
	}

	// Read some bytes from this file and check if it is text to avoid adding
	// binary files
	buf := make([]byte, 128)
	_, err = file.Read(buf)
	if err != nil && err != io.EOF {
		log.Infof("Discovered log file %s could not be read: %v", logPath, err)
		return false
	}

	if !utf8.Valid(buf) {
		log.Infof("Discovered log file %s is not a text file", logPath)
		return false
	}

	defer file.Close()
	return true
}

func (p *ProcessLogConfigProvider) isFileReadable(logPath string) bool {
	if _, found := p.unreadableFilesCache.Get(logPath); found {
		return false
	}

	if !isFileReadable(logPath) {
		p.unreadableFilesCache.Add(logPath, struct{}{})
		return false
	}

	return true
}

func (p *ProcessLogConfigProvider) processEventsInner(evBundle workloadmeta.EventBundle, verifyReadable bool) integration.ConfigChanges {
	p.mu.Lock()
	defer p.mu.Unlock()

	changes := integration.ConfigChanges{}

	for _, event := range evBundle.Events {
		process, ok := event.Entity.(*workloadmeta.Process)
		if !ok {
			continue
		}

		switch event.Type {
		case workloadmeta.EventTypeSet:
			// First, decrement refcounts for existing service IDs associated with this PID
			existingServiceIDs := p.pidToServiceIDs[process.Pid]

			for _, serviceLogKey := range existingServiceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					ref.refCount--
				}
			}

			// Clear the existing service IDs for this PID
			delete(p.pidToServiceIDs, process.Pid)

			// Ignore containers since log files inside them usually can't be
			// accessed from here since they are in a different namespace.
			if process.Service != nil && process.ContainerID == "" {
				// Process new log files
				newServiceIDs := []string{}
				for _, logFile := range process.Service.LogFiles {
					serviceLogKey := p.generateServiceLogKey(process.Service.GeneratedName, logFile)
					newServiceIDs = append(newServiceIDs, serviceLogKey)

					if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
						// Config already exists, just increment reference count
						ref.refCount++
					} else {
						if verifyReadable && !p.isFileReadable(logFile) {
							continue
						}

						log.Infof("Discovered log file %s", logFile)

						// Create new config and reference
						config, err := p.buildConfig(process, logFile, serviceLogKey)
						if err != nil {
							log.Warnf("could not build log config for process %s and file %s: %v", process.EntityID, logFile, err)
							continue
						}

						p.serviceLogRefs[serviceLogKey] = &serviceLogRef{
							refCount: 1,
							config:   config,
						}

						// Add to config cache and schedule
						changes.ScheduleConfig(config)
					}
				}

				// Store the new service IDs for this PID
				if len(newServiceIDs) > 0 {
					p.pidToServiceIDs[process.Pid] = newServiceIDs
				}
			}

			for _, serviceLogKey := range existingServiceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					if ref.refCount <= 0 {
						changes.UnscheduleConfig(ref.config)
						delete(p.serviceLogRefs, serviceLogKey)
					}
				}
			}

		case workloadmeta.EventTypeUnset:
			// Get service IDs associated with this PID
			serviceIDs := p.pidToServiceIDs[process.Pid]
			for _, serviceLogKey := range serviceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					ref.refCount--

					if ref.refCount <= 0 {
						// Reference count reached zero, unschedule and cleanup
						changes.UnscheduleConfig(ref.config)
						delete(p.serviceLogRefs, serviceLogKey)
					}
				}
			}

			// Remove the PID entry
			delete(p.pidToServiceIDs, process.Pid)
		}
	}

	return changes
}

func (p *ProcessLogConfigProvider) buildConfig(process *workloadmeta.Process, logFile, serviceLogKey string) (integration.Config, error) {
	logConfig := map[string]interface{}{
		"type":    "file",
		"path":    logFile,
		"service": process.Service.DDService,
		"source":  process.Service.GeneratedName,
	}

	data, err := json.Marshal([]map[string]interface{}{logConfig})
	if err != nil {
		return integration.Config{}, fmt.Errorf("could not marshal log config: %w", err)
	}

	return integration.Config{
		Name:       fmt.Sprintf("process-%s-%s", process.Service.GeneratedName, strings.ReplaceAll(logFile, "/", "_")),
		LogsConfig: data,
		Provider:   processLogProviderName,
		Source:     "process-log:" + process.Service.GeneratedName,
		ServiceID:  fmt.Sprintf("process_log://%s", serviceLogKey),
	}, nil
}

// GetConfigErrors returns a map of configuration errors, which is always empty for this provider.
func (p *ProcessLogConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
