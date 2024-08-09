// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration creates a launcher to track logs from integrations
package integration

import (
	"os"
	"strings"

	ddLog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// Launcher checks for launcher integrations, creates files for integrations to
// write logs to, then creates file sources for the file launcher to tail
type Launcher struct {
	sources              *sources.LogSources
	addedSources         chan *sources.LogSource
	stop                 chan struct{}
	runPath              string
	integrationsLogsChan chan integrations.IntegrationLog
	integrationToFile    map[string]string
	// writeLogToFile is used as a function pointer so it can be overridden in
	// testing to make deterministic tests
	writeFunction func(filepath, log string) error
}

// NewLauncher returns a new launcher
func NewLauncher(sources *sources.LogSources, integrationsLogsComp integrations.Component) *Launcher {
	return &Launcher{
		sources:              sources,
		runPath:              pkgConfig.Datadog().GetString("logs_config.run_path"),
		stop:                 make(chan struct{}),
		integrationsLogsChan: integrationsLogsComp.Subscribe(),
		integrationToFile:    make(map[string]string),
		writeFunction:        writeLogToFile,
	}
}

// Start starts the launcher and launches the run loop in a go function
func (s *Launcher) Start(sourceProvider launchers.SourceProvider, _ pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
	s.addedSources, _ = sourceProvider.SubscribeForType(config.IntegrationType)

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
		case source := <-s.addedSources:
			// Send logs configurations to the file launcher to tail, it will handle
			// tailer lifecycle, file rotation, etc.
			filepath, err := s.createFile(source)
			if err != nil {
				ddLog.Warn("Failed to create integration log file: ", err)
				continue
			}

			filetypeSource := s.makeFileSource(source, filepath)
			s.sources.AddSource(filetypeSource)

			// file to write the incoming logs to
			s.integrationToFile[source.Name] = filepath
		case log := <-s.integrationsLogsChan:
			// Integrations will come in the form of: check_name:instance_config_hash
			integrationSplit := strings.Split(log.IntegrationID, ":")
			integrationName := integrationSplit[0]
			filepath := s.integrationToFile[integrationName]

			err := s.ensureFileSize(filepath)
			if err != nil {
				ddLog.Warn("Failed to get file size: ", err)
				continue
			}

			err = s.writeFunction(filepath, log.Log)
			if err != nil {
				ddLog.Warn("Error writing log to file: ", err)
			}
		case <-s.stop:
			return
		}
	}
}

// writeLogToFile is used as a function pointer
func writeLogToFile(filepath, log string) error {
	file, err := os.OpenFile(filepath, os.O_WRONLY, 0644)
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

	return nil
}

// makeFileSource Turns an integrations source into a logsSource
func (s *Launcher) makeFileSource(source *sources.LogSource, filepath string) *sources.LogSource {
	fileSource := sources.NewLogSource(source.Name, &config.LogsConfig{
		Type:        config.FileType,
		TailingMode: source.Config.TailingMode,
		Path:        filepath,
		Name:        source.Config.Name,
		Source:      source.Config.Source,
		Tags:        source.Config.Tags,
	})

	return fileSource
}

// TODO Change file naming to reflect ID once logs from go interfaces gets merged.
// createFile creates a file for the logsource
func (s *Launcher) createFile(source *sources.LogSource) (string, error) {
	directory, filepath := s.integrationLogFilePath(*source)

	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return "", err
	}

	file, err := os.Create(filepath)
	if err != nil {
		return "", nil
	}
	defer file.Close()

	return filepath, nil
}

// integrationLogFilePath returns a directory and file to use for an integration log file
func (s *Launcher) integrationLogFilePath(source sources.LogSource) (string, string) {
	fileName := source.Config.Name + ".log"
	directoryComponents := []string{s.runPath, "integrations", source.Config.Service}
	directory := strings.Join(directoryComponents, "/")
	filepath := strings.Join([]string{directory, fileName}, "/")

	return directory, filepath
}

// ensureFileSize enforces the max file size for files integrations logs
// files. Files over the set size will be deleted and remade.
func (s *Launcher) ensureFileSize(filepath string) error {
	maxFileSizeSetting := pkgConfig.Datadog().GetInt64("logs_config.integrations_logs_files_max_size")
	maxFileSizeBytes := maxFileSizeSetting * 1024 * 1024

	fi, err := os.Stat(filepath)
	if err != nil {
		return err
	}

	if fi.Size() > int64(maxFileSizeBytes) {
		err := os.Remove(filepath)
		if err != nil {
			if os.IsNotExist(err) {
				ddLog.Warn("File does not exist, creating new one: ", err)
			} else {
				ddLog.Warn("Error deleting file: ", err)
			}
		} else {
			ddLog.Info("Successfully deleted oversize log file, creating new one.")
		}

		file, err := os.Create(filepath)
		if err != nil {
			return err
		}
		defer file.Close()
	}

	return nil
}
