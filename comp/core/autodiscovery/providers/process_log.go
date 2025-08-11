// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/discovery"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type serviceLogRef struct {
	refCount int
	config   integration.Config
}

type processLogConfigProvider struct {
	workloadmetaStore    workloadmeta.Component
	serviceLogRefs       map[string]*serviceLogRef
	pidToServiceIDs      map[int32][]string
	unreadableFilesCache *simplelru.LRU[string, struct{}]
	mu                   sync.RWMutex
}

var _ types.ConfigProvider = &processLogConfigProvider{}
var _ types.StreamingConfigProvider = &processLogConfigProvider{}

// NewProcessLogConfigProvider returns a new ConfigProvider subscribed to process events
func NewProcessLogConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ *telemetry.Store) (types.ConfigProvider, error) {
	cache, err := simplelru.NewLRU[string, struct{}](128, nil)
	if err != nil {
		return nil, err
	}
	return &processLogConfigProvider{
		workloadmetaStore:    wmeta,
		serviceLogRefs:       make(map[string]*serviceLogRef),
		pidToServiceIDs:      make(map[int32][]string),
		unreadableFilesCache: cache,
	}, nil
}

// String returns a string representation of the ProcessLogConfigProvider
func (p *processLogConfigProvider) String() string {
	return names.ProcessLog
}

// Stream starts listening to workloadmeta to generate configs as they come
func (p *processLogConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)

	filter := workloadmeta.NewFilterBuilder().
		AddKindWithEntityFilter(workloadmeta.KindProcess, func(e workloadmeta.Entity) bool {
			process, ok := e.(*workloadmeta.Process)
			if !ok {
				return false
			}

			// Ignore containers since log files inside them usually can't be
			// accessed from here since they are in a different namespace.
			return process.Service != nil && process.ContainerID == ""
		}).
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

				evBundle.Acknowledge()
				outCh <- p.processEvents(evBundle)
			}
		}
	}()

	return outCh
}

func (p *processLogConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	return p.processEventsInner(evBundle, true)
}

func checkFileReadable(logPath string) error {
	file, err := os.Open(logPath)
	if err != nil {
		log.Infof("Discovered log file %s could not be opened: %v", logPath, err)
		return err
	}

	defer file.Close()

	// Read some bytes from this file and check if it is text to avoid adding
	// binary files
	buf := make([]byte, 128)
	_, err = file.Read(buf)
	if err != nil && err != io.EOF {
		log.Infof("Discovered log file %s could not be read: %v", logPath, err)
		return err
	}

	if !utf8.Valid(buf) {
		log.Infof("Discovered log file %s is not a text file", logPath)
		return fmt.Errorf("file is not a text file")
	}

	return nil
}

func (p *processLogConfigProvider) isFileReadable(logPath string) bool {
	if _, found := p.unreadableFilesCache.Get(logPath); found {
		return false
	}

	err := checkFileReadable(logPath)
	if err != nil {
		// We want to display permissions errors in the agent status.  Other
		// errors such as file not found are likely due to the log file having
		// gone away and are not actionable.
		if errors.Is(err, os.ErrPermission) {
			message := fmt.Sprintf("Discovered log file %s could not be opened due to lack of permissions", logPath)
			discovery.AddWarning(logPath, err, message)
			status.AddGlobalWarning(logPath, message)
		}

		oldestPath, _, _ := p.unreadableFilesCache.GetOldest()
		evicted := p.unreadableFilesCache.Add(logPath, struct{}{})
		// We don't want to keep the number of warnings growing forever, so
		// only keep warnings for files in our LRU cache.
		if evicted {
			discovery.RemoveWarning(oldestPath)
			status.RemoveGlobalWarning(oldestPath)
		}

		return false
	}

	// Remove any existing warning for this file, since it is readable. Note that we won't get here
	// for an existing file until it is evicted from the LRU cache.
	discovery.RemoveWarning(logPath)
	status.RemoveGlobalWarning(logPath)

	return true
}

func (p *processLogConfigProvider) processEventsInner(evBundle workloadmeta.EventBundle, verifyReadable bool) integration.ConfigChanges {
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
			// The set of logs monitored by this service may change, so we need
			// to handle deleting existing logs too. First, decrement refcounts
			// for existing service IDs associated with this PID. Any logs still
			// present will get their refcount increased in the loop.
			existingServiceIDs := p.pidToServiceIDs[process.Pid]

			for _, serviceLogKey := range existingServiceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					ref.refCount--
				}
			}

			// Clear the existing service IDs for this PID, we will re-add them if this
			// process still has logs.
			delete(p.pidToServiceIDs, process.Pid)

			newServiceIDs := []string{}
			for _, logFile := range process.Service.LogFiles {
				newServiceIDs = append(newServiceIDs, logFile)

				if ref, exists := p.serviceLogRefs[logFile]; exists {
					ref.refCount++
				} else {
					if verifyReadable && !p.isFileReadable(logFile) {
						continue
					}

					log.Infof("Discovered log file %s", logFile)

					// Create new config and reference
					config, err := p.buildConfig(process, logFile)
					if err != nil {
						log.Warnf("could not build log config for process %s and file %s: %v", process.EntityID, logFile, err)
						continue
					}

					p.serviceLogRefs[logFile] = &serviceLogRef{
						refCount: 1,
						config:   config,
					}

					changes.ScheduleConfig(config)
				}
			}

			if len(newServiceIDs) > 0 {
				p.pidToServiceIDs[process.Pid] = newServiceIDs
			}

			// Unschedule any logs that are no longer present
			for _, serviceLogKey := range existingServiceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					if ref.refCount <= 0 {
						changes.UnscheduleConfig(ref.config)
						delete(p.serviceLogRefs, serviceLogKey)
					}
				}
			}

		case workloadmeta.EventTypeUnset:
			serviceIDs := p.pidToServiceIDs[process.Pid]
			for _, serviceLogKey := range serviceIDs {
				if ref, exists := p.serviceLogRefs[serviceLogKey]; exists {
					ref.refCount--

					if ref.refCount <= 0 {
						changes.UnscheduleConfig(ref.config)
						delete(p.serviceLogRefs, serviceLogKey)
					}
				}
			}

			delete(p.pidToServiceIDs, process.Pid)
		}
	}

	return changes
}

// getServiceName returns the name of the service to be used in the log config.
func getServiceName(service *workloadmeta.Service) string {
	if len(service.TracerMetadata) > 0 {
		return service.TracerMetadata[0].ServiceName
	}

	if service.DDService != "" {
		return service.DDService
	}

	return service.GeneratedName
}

// getSource returns the source to be used in the log config. This needs to
// match the integration pipelines, see
// https://app.datadoghq.com/logs/pipelines/pipeline/library. For now, this has
// some handling for some common cases, until a better solution is available.
func getSource(service *workloadmeta.Service) string {
	source := service.GeneratedName

	// Binary name differs from the integration name
	if source == "apache2" {
		return "apache"
	}
	if source == "postgres" {
		return "postgresql"
	}
	if source == "catalina" {
		return "tomcat"
	}

	// The generated name may be the WSGI application name
	if service.GeneratedNameSource == "gunicorn" {
		return "gunicorn"
	}

	return source
}

func getIntegrationName(logFile string) string {
	return fmt.Sprintf("%s:%s", names.ProcessLog, logFile)
}

func getServiceID(logFile string) string {
	return fmt.Sprintf("%s://%s", names.ProcessLog, logFile)
}

func (p *processLogConfigProvider) buildConfig(process *workloadmeta.Process, logFile string) (integration.Config, error) {
	name := getServiceName(process.Service)
	source := getSource(process.Service)

	logConfig := map[string]interface{}{
		"type":    "file",
		"path":    logFile,
		"service": name,
		"source":  source,
	}

	data, err := json.Marshal([]map[string]interface{}{logConfig})
	if err != nil {
		return integration.Config{}, fmt.Errorf("could not marshal log config: %w", err)
	}

	integrationName := getIntegrationName(logFile)
	return integration.Config{
		Name:       integrationName,
		LogsConfig: data,
		Provider:   names.ProcessLog,
		Source:     integrationName,
		ServiceID:  getServiceID(logFile),
	}, nil
}

// GetConfigErrors returns a map of configuration errors, which is always empty for this provider.
func (p *processLogConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
