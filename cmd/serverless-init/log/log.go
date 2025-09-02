// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log is responsible for settings around logging output from customer functions
// to be sent to Datadog (logs monitoring product).
// It does *NOT* control the internal debug logging of the agent.
package log

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultFlushTimeout = 5 * time.Second
	logEnabledEnvVar    = "DD_LOGS_ENABLED"
	envVarTailFilePath  = "DD_SERVERLESS_LOG_PATH"
	aasInstanceTailing  = "DD_AAS_INSTANCE_LOGGING_ENABLED"
	sourceEnvVar        = "DD_SOURCE"
	sourceName          = "Datadog Agent"
)

// Config holds the log configuration
type Config struct {
	FlushTimeout time.Duration
	Channel      chan *logConfig.ChannelMessage
	source       string
	IsEnabled    bool
}

// CreateConfig builds and returns a log config
func CreateConfig(origin string) *Config {
	var source string
	if source = strings.ToLower(os.Getenv(sourceEnvVar)); source == "" {
		source = origin
	}
	return &Config{
		FlushTimeout: defaultFlushTimeout,
		// Use a buffered channel with size 10000
		Channel:   make(chan *logConfig.ChannelMessage, 10000),
		source:    source,
		IsEnabled: isEnabled(os.Getenv(logEnabledEnvVar)),
	}
}

// SetupLogAgent creates the log agent and sets the base tags
func SetupLogAgent(conf *Config, tags map[string]string, tagger tagger.Component, compression logscompression.Component, origin string) logsAgent.ServerlessLogsAgent {
	// When Azure App Service instance tailing is enabled, ensure we only open the most
	// recently modified match. Respect existing global env overrides if already set.
	if isInstanceTailingEnabled() {
		if os.Getenv("DD_LOGS_CONFIG_FILE_WILDCARD_SELECTION_MODE") == "" {
			_ = os.Setenv("DD_LOGS_CONFIG_FILE_WILDCARD_SELECTION_MODE", "by_modification_time")
			ddlog.Debug("AAS instance tailing: setting default DD_LOGS_CONFIG_FILE_WILDCARD_SELECTION_MODE=by_modification_time")
		} else {
			ddlog.Debugf("AAS instance tailing: using existing DD_LOGS_CONFIG_FILE_WILDCARD_SELECTION_MODE=%q", os.Getenv("DD_LOGS_CONFIG_FILE_WILDCARD_SELECTION_MODE"))
		}
		if os.Getenv("DD_LOGS_CONFIG_OPEN_FILES_LIMIT") == "" {
			_ = os.Setenv("DD_LOGS_CONFIG_OPEN_FILES_LIMIT", "1")
			ddlog.Debug("AAS instance tailing: setting default DD_LOGS_CONFIG_OPEN_FILES_LIMIT=1")
		} else {
			ddlog.Debugf("AAS instance tailing: using existing DD_LOGS_CONFIG_OPEN_FILES_LIMIT=%q", os.Getenv("DD_LOGS_CONFIG_OPEN_FILES_LIMIT"))
		}
	}

	logsAgent, _ := serverlessLogs.SetupLogAgent(conf.Channel, sourceName, conf.source, tagger, compression)

	tagsArray := serverlessTag.MapToArray(tags)

	addFileTailing(logsAgent, conf.source, tagsArray, origin)

	serverlessLogs.SetLogsTags(tagsArray)
	return logsAgent
}

func addFileTailing(logsAgent logsAgent.ServerlessLogsAgent, source string, tags []string, origin string) {

	appServiceDefaultLoggingEnabled := origin == "appservice" && isInstanceTailingEnabled()
	// The Azure App Service log volume is shared across all instances. This leads to every instance tailing the same files.
	// To avoid this, we want to add the azure instance ID to the filepath so each instance tails their respective system log files.
	// Users can also add $COMPUTERNAME to their custom files to achieve the same result.
	if appServiceDefaultLoggingEnabled {
		pattern := buildAASInstancePattern()
		ddlog.Debugf("AAS instance tailing: using pattern %q", pattern)
		limit := effectiveOpenFilesLimit()
		debugLogFileSelection(pattern, limit)

		src := sources.NewLogSource("aas-instance-file-tail", &logConfig.LogsConfig{
			Type:    logConfig.FileType,
			Path:    pattern,
			Service: os.Getenv("DD_SERVICE"),
			Tags:    tags,
			Source:  source,
		})
		logsAgent.GetSources().AddSource(src)
		// If we are not in Azure or the aas instance env var is not set, we fall back to the previous behavior
	} else if filePath, set := os.LookupEnv(envVarTailFilePath); set {
		src := sources.NewLogSource("serverless-file-tail", &logConfig.LogsConfig{
			Type:    logConfig.FileType,
			Path:    filePath,
			Service: os.Getenv("DD_SERVICE"),
			Tags:    tags,
			Source:  source,
		})
		logsAgent.GetSources().AddSource(src)
	}
}

// buildAASInstancePattern builds the glob for Azure App Service instance logs.
// Priority:
// 1) DD_AAS_INSTANCE_LOG_GLOB (full glob, supports $COMPUTERNAME)
// 2) DD_SERVICE (service basename hint)
// 3) WEBSITE_SITE_NAME (App Service app name)
// 4) default: /home/LogFiles/*$COMPUTERNAME*.log
func buildAASInstancePattern() string {
	if g := os.Getenv("DD_AAS_INSTANCE_LOG_GLOB"); g != "" {
		return os.ExpandEnv(g)
	}
	if svc := strings.TrimSpace(os.Getenv("DD_SERVICE")); svc != "" {
		return os.ExpandEnv("/home/LogFiles/*$COMPUTERNAME*_" + svc + "*.log")
	}
	if site := strings.TrimSpace(os.Getenv("WEBSITE_SITE_NAME")); site != "" {
		return os.ExpandEnv("/home/LogFiles/*$COMPUTERNAME*_" + site + "*.log")
	}
	return os.ExpandEnv("/home/LogFiles/*$COMPUTERNAME*.log")
}

// effectiveOpenFilesLimit returns the intended open files limit for AAS instance tailing.
// Source: DD_LOGS_CONFIG_OPEN_FILES_LIMIT, default 1.
func effectiveOpenFilesLimit() int {
	if v := os.Getenv("DD_LOGS_CONFIG_OPEN_FILES_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// debugLogFileSelection logs all files matching the pattern and the top-N by modification time.
func debugLogFileSelection(pattern string, limit int) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		ddlog.Debugf("AAS instance tailing: glob error for pattern %q: %v", pattern, err)
		return
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}
	files := make([]fileInfo, 0, len(matches))
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			files = append(files, fileInfo{path: p, modTime: st.ModTime()})
		}
	}
	// Sort by most recent first
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })

	all := make([]string, 0, len(files))
	for _, f := range files {
		all = append(all, f.path)
	}
	ddlog.Debugf("AAS instance tailing: %d files match %q: %v", len(all), pattern, all)

	if limit <= 0 {
		limit = 1
	}
	if limit > len(files) {
		limit = len(files)
	}
	selected := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		selected = append(selected, files[i].path)
	}
	ddlog.Debugf("AAS instance tailing: selecting up to %d most recently modified: %v", limit, selected)
}

func isEnabled(envValue string) bool {
	return strings.ToLower(envValue) == "true"
}

func isInstanceTailingEnabled() bool {
	val := strings.ToLower(os.Getenv(aasInstanceTailing))
	return val == "true" || val == "1"
}
