// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration creates a launcher to track logs from integrations
package integration

import (
	"os"
	"path/filepath"
	"strings"

	ddLog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

var endOfLine = []byte{'\n'}

// Launcher checks for launcher integrations, creates files for integrations to
// write logs to, then creates file sources for the file launcher to tail
type Launcher struct {
	sources              *sources.LogSources
	addedConfigs         chan integrations.IntegrationConfig
	stop                 chan struct{}
	runPath              string
	integrationsLogsChan chan integrations.IntegrationLog
	integrationToFile    map[string]string
	// writeLogToFile is used as a function pointer, so it can be overridden in
	// testing to make deterministic tests
	writeFunction func(logFilePath, log string) error
}

// NewLauncher creates and returns an integrations launcher, and creates the
// path for integrations files to run in
func NewLauncher(sources *sources.LogSources, integrationsLogsComp integrations.Component) *Launcher {
	runPath := filepath.Join(pkgconfigsetup.Datadog().GetString("logs_config.run_path"), "integrations")
	err := os.MkdirAll(runPath, 0755)

	if err != nil {
		ddLog.Warn("Unable to create integrations logs directory:", err)
	}

	return &Launcher{
		sources:              sources,
		runPath:              runPath,
		stop:                 make(chan struct{}),
		integrationsLogsChan: integrationsLogsComp.Subscribe(),
		addedConfigs:         integrationsLogsComp.SubscribeIntegration(),
		integrationToFile:    make(map[string]string),
		writeFunction:        writeLogToFile,
	}
}

// Start starts the launcher and launches the run loop in a go function
func (s *Launcher) Start(_ launchers.SourceProvider, _ pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
	go s.run()
}

// Stop stops the launcher
func (s *Launcher) Stop() {
	s.stop <- struct{}{}
}

// run checks if there are new files to tail and tails them
func (s *Launcher) run() {
	for {
		select {
		case cfg := <-s.addedConfigs:

			sources, err := ad.CreateSources(cfg.Config)
			if err != nil {
				ddLog.Warn("Failed to create source ", err)
				continue
			}

			for _, source := range sources {
				// TODO: integrations should only be allowed to have one IntegrationType config.
				if source.Config.Type == config.IntegrationType {
					logFilePath, err := s.createFile(cfg.IntegrationID)
					if err != nil {
						ddLog.Warn("Failed to create integration log file: ", err)
						continue
					}
					filetypeSource := s.makeFileSource(source, logFilePath)
					s.sources.AddSource(filetypeSource)

					// file to write the incoming logs to
					s.integrationToFile[cfg.IntegrationID] = logFilePath
				}
			}

		case log := <-s.integrationsLogsChan:
			logFilePath := s.integrationToFile[log.IntegrationID]

			err := s.ensureFileSize(logFilePath)
			if err != nil {
				ddLog.Warn("Failed to get file size: ", err)
				continue
			}

			err = s.writeFunction(logFilePath, log.Log)
			if err != nil {
				ddLog.Warn("Error writing log to file: ", err)
			}
		case <-s.stop:
			return
		}
	}
}

// writeLogToFile is used as a function pointer
func writeLogToFile(logFilePath, log string) error {
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		ddLog.Warn("Failed to open file to write log to: ", err)
		return err
	}

	defer file.Close()

	_, err = file.WriteString(log)
	if err != nil {
		ddLog.Warn("Failed to write integration log to file: ", err)
		return err
	}
	if _, err = file.Write(endOfLine); err != nil {
		ddLog.Warn("Failed to write integration log to file: ", err)
		return err
	}
	return nil
}

// makeFileSource Turns an integrations source into a logsSource
func (s *Launcher) makeFileSource(source *sources.LogSource, logFilePath string) *sources.LogSource {
	fileSource := sources.NewLogSource(source.Name, &config.LogsConfig{
		Type:        config.FileType,
		TailingMode: source.Config.TailingMode,
		Path:        logFilePath,
		Name:        source.Config.Name,
		Source:      source.Config.Source,
		Service:     source.Config.Service,
		Tags:        source.Config.Tags,
	})

	fileSource.SetSourceType(sources.IntegrationSourceType)
	return fileSource
}

// TODO Change file naming to reflect ID once logs from go interfaces gets merged.
// createFile creates a file for the logsource
func (s *Launcher) createFile(id string) (string, error) {
	logFilePath := s.integrationLogFilePath(id)

	file, err := os.Create(logFilePath)
	if err != nil {
		return "", nil
	}
	defer file.Close()

	return logFilePath, nil
}

// integrationLoglogFilePath returns a file path to use for an integration log file
func (s *Launcher) integrationLogFilePath(id string) string {
	fileName := strings.ReplaceAll(id, " ", "-")
	fileName = strings.ReplaceAll(fileName, ":", "_") + ".log"
	logFilePath := filepath.Join(s.runPath, fileName)

	return logFilePath
}

// ensureFileSize enforces the max file size for files integrations logs
// files. Files over the set size will be deleted and remade.
func (s *Launcher) ensureFileSize(logFilePath string) error {
	maxFileSizeSetting := pkgconfigsetup.Datadog().GetInt64("logs_config.integrations_logs_files_max_size")
	maxFileSizeBytes := maxFileSizeSetting * 1024 * 1024

	fi, err := os.Stat(logFilePath)
	if err != nil {
		return err
	}

	if fi.Size() > int64(maxFileSizeBytes) {
		err := os.Remove(logFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				ddLog.Warn("File does not exist, creating new one: ", err)
			} else {
				ddLog.Warn("Error deleting file: ", err)
			}
		} else {
			ddLog.Info("Successfully deleted oversize log file, creating new one.")
		}

		file, err := os.Create(logFilePath)
		if err != nil {
			return err
		}
		defer file.Close()
	}

	return nil
}
