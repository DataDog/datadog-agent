// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/discovery"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	privilegedlogsclient "github.com/DataDog/datadog-agent/pkg/privileged-logs/client"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type serviceLogRef struct {
	refCount int
	config   integration.Config
}

type processLogConfigProvider struct {
	workloadmetaStore       workloadmeta.Component
	tagger                  tagger.Component
	logsFilters             workloadfilter.FilterBundle
	serviceLogRefs          map[string]*serviceLogRef
	pidToServiceIDs         map[int32][]string
	unreadableFilesCache    *simplelru.LRU[string, struct{}]
	validIntegrationSources map[string]bool
	mu                      sync.RWMutex
	excludeAgent            bool
}

var _ types.ConfigProvider = &processLogConfigProvider{}
var _ types.StreamingConfigProvider = &processLogConfigProvider{}

func addSources(sources map[string]bool, path string, sourcePattern *regexp.Regexp) {
	file, err := os.Open(path)
	if err != nil {
		log.Debugf("Could not read %s: %v", path, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if matches := sourcePattern.FindStringSubmatch(scanner.Text()); len(matches) > 1 {
			source := matches[1]
			if _, ok := sources[source]; ok {
				continue
			}

			sources[source] = true
			log.Tracef("Discovered integration source: %s from %s", source, path)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debugf("Could not read %s: %v", path, err)
	}
}

// discoverIntegrationSources scans configuration directories to find valid
// integration log sources by parsing conf.yaml.example files (or conf.yaml
// files) and extracting log source names.
func discoverIntegrationSources() map[string]bool {
	sources := make(map[string]bool)

	// Build search paths similar to LoadComponents
	searchPaths := []string{
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		pkgconfigsetup.Datadog().GetString("confd_path"),
	}

	log.Tracef("Discovering integration sources from paths: %v", searchPaths)

	// Pattern to match source lines like "source: nginx" (including commented-out lines).
	sourcePattern := regexp.MustCompile(`^#?\s*source:\s*"?(.+?)"?\s*$`)

	for _, searchPath := range searchPaths {
		if searchPath == "" {
			continue
		}

		err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Continue walking, don't fail on individual errors
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() != "conf.yaml.example" && d.Name() != "conf.yaml" {
				return nil
			}

			addSources(sources, path, sourcePattern)

			return nil
		})

		if err != nil {
			log.Debugf("Error discovering integration sources in %s: %v", searchPath, err)
		}
	}

	log.Debugf("Discovered %d integration sources", len(sources))

	if len(sources) > 0 {
		// Agent sources need to be special cased since they don't come with log
		// examples.  If we don't have any integration sources at all (likely
		// due to errors), don't add the agent sources, but just allow the
		// source selection function to allow all sources.
		for _, agentName := range agentProcessNames {
			sources[agentName] = true
		}
	}

	return sources
}

// NewProcessLogConfigProvider returns a new ConfigProvider subscribed to process events
func NewProcessLogConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, tagger tagger.Component, filter workloadfilter.Component, _ *telemetry.Store) (types.ConfigProvider, error) {
	cache, err := simplelru.NewLRU[string, struct{}](128, nil)
	if err != nil {
		return nil, err
	}

	// Discover available integration sources
	validSources := discoverIntegrationSources()

	return &processLogConfigProvider{
		workloadmetaStore:       wmeta,
		tagger:                  tagger,
		logsFilters:             filter.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELLogs, workloadfilter.ProcessCELGlobal}}),
		serviceLogRefs:          make(map[string]*serviceLogRef),
		pidToServiceIDs:         make(map[int32][]string),
		unreadableFilesCache:    cache,
		validIntegrationSources: validSources,
		excludeAgent:            pkgconfigsetup.Datadog().GetBool("logs_config.process_exclude_agent"),
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
	// Check readability with the privileged logs client to match what the
	// log tailer uses.  That client can use the privileged logs module in
	// system-probe if it is available.
	file, err := privilegedlogsclient.Open(logPath)
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
		return errors.New("file is not a text file")
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

var agentProcessNames = []string{
	"agent",
	"process-agent",
	"trace-agent",
	"security-agent",
	"system-probe",
	"privateactionrunner",
}

func isAgentProcess(process *workloadmeta.Process) bool {
	// Check if the process name matches any of the known agent process names;
	// we may not be able to make assumptions about the executable paths.
	return slices.Contains(agentProcessNames, process.Name)
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
			if p.excludeAgent && isAgentProcess(process) {
				log.Debugf("Excluding agent process %d (comm=%s) from process log collection", process.Pid, process.Comm)
				continue
			}

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

			var filterProcess *workloadfilter.Process
			if p.logsFilters != nil {
				filterProcess = workloadmetafilter.CreateProcess(process)
			}

			newServiceIDs := []string{}
			for _, logFile := range process.Service.LogFiles {
				if filterProcess != nil {
					filterProcess.SetLogFile(logFile)
					if p.logsFilters.IsExcluded(filterProcess) {
						log.Debugf("Process %d log file %s excluded by CEL filter", process.Pid, logFile)
						continue
					}
				}

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
	if service.UST.Service != "" {
		return service.UST.Service
	}

	return service.GeneratedName
}

var generatedNameToSource = map[string]string{
	"apache2":           "apache",
	"catalina":          "tomcat",
	"clickhouse-server": "clickhouse",
	"cockroach":         "cockroachdb",
	"kafka.Kafka":       "kafka",
	"postgres":          "postgresql",
	"mongod":            "mongodb",
	"mysqld":            "mysql",
	"redis-server":      "redis",
	"slapd":             "openldap",
	"tailscaled":        "tailscale",
}

var languageToSource = map[languagemodels.LanguageName]string{
	languagemodels.Python: "python",
	languagemodels.Go:     "go",
	languagemodels.Java:   "java",
	languagemodels.Node:   "nodejs",
	languagemodels.Ruby:   "ruby",
	languagemodels.Dotnet: "csharp",
}

func fixupGeneratedName(generatedName string) string {
	if replacement, ok := generatedNameToSource[generatedName]; ok {
		return replacement
	}

	// Handle special prefixes
	if strings.HasPrefix(generatedName, "org.elasticsearch.") {
		return "elasticsearch"
	}
	if strings.HasPrefix(generatedName, "org.sonar.") {
		return "sonarqube"
	}

	return generatedName
}

// getSource returns the source to be used in the log config. This needs to
// match the integration pipelines, see
// https://app.datadoghq.com/logs/pipelines/pipeline/library.
func (p *processLogConfigProvider) getSource(process *workloadmeta.Process) string {
	service := process.Service
	candidate := fixupGeneratedName(service.GeneratedName)

	if p.validIntegrationSources[candidate] {
		return candidate
	}

	// If we don't know what the valid sources are, just use the candidate as is.
	if len(p.validIntegrationSources) == 0 {
		return candidate
	}

	// For gunicorn applications, the generated name may be the WSGI application
	// name, so check the source of the generated name and prefer the gunicorn
	// log source over the generic Python log source.
	if service.GeneratedNameSource == "gunicorn" {
		return "gunicorn"
	}

	// If we have a language-specific parser, use that as the source.
	if process.Language == nil {
		return candidate
	}
	if source, ok := languageToSource[process.Language.Name]; ok {
		return source
	}

	return candidate
}

func getIntegrationName(logFile string) string {
	return fmt.Sprintf("%s:%s", names.ProcessLog, logFile)
}

func getServiceID(logFile string) string {
	return fmt.Sprintf("%s://%s", names.ProcessLog, logFile)
}

func (p *processLogConfigProvider) getProcessTags(pid int32) ([]string, error) {
	if p.tagger == nil {
		return nil, errors.New("tagger not available")
	}
	entityID := taggertypes.NewEntityID(taggertypes.Process, strconv.Itoa(int(pid)))
	return p.tagger.Tag(entityID, taggertypes.HighCardinality)
}

func (p *processLogConfigProvider) buildConfig(process *workloadmeta.Process, logFile string) (integration.Config, error) {
	name := getServiceName(process.Service)
	source := p.getSource(process)

	logConfig := map[string]interface{}{
		"type":    "file",
		"path":    logFile,
		"service": name,
		"source":  source,
	}

	if tags, err := p.getProcessTags(process.Pid); err == nil && len(tags) > 0 {
		logConfig["tags"] = tags
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
