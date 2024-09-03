// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/sbom/scanner"
)

// Match .yaml and .yml to ship configuration files in the flare.
var cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func (f *flare) collectLogsFiles(fb types.FlareBuilder) error {
	logFile := f.config.GetString("log_file")
	if logFile == "" {
		logFile = f.params.defaultLogFile
	}

	jmxLogFile := f.config.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = f.params.defaultJMXLogFile
	}

	dogstatsdLogFile := f.config.GetString("dogstatsd_log_file")
	if dogstatsdLogFile == "" {
		dogstatsdLogFile = f.params.defaultDogstatsdLogFile
	}

	shouldIncludeFunc := func(path string) bool {
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	}

	f.log.Flush()
	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFile), "logs", shouldIncludeFunc)          //nolint:errcheck
	fb.CopyDirToWithoutScrubbing(filepath.Dir(jmxLogFile), "logs", shouldIncludeFunc)       //nolint:errcheck
	fb.CopyDirToWithoutScrubbing(filepath.Dir(dogstatsdLogFile), "logs", shouldIncludeFunc) //nolint:errcheck
	return nil
}

func (f *flare) collectConfigFiles(fb types.FlareBuilder) error {
	confSearchPaths := map[string]string{
		"":        f.config.GetString("confd_path"),
		"dist":    filepath.Join(f.params.distPath, "conf.d"),
		"checksd": f.params.pythonChecksPath,
	}

	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool { //nolint:errcheck
			// ignore .example file
			if filepath.Ext(path) == ".example" {
				return false
			}

			firstSuffix := []byte(getFirstSuffix(path))
			ext := []byte(filepath.Ext(path))
			if cnfFileExtRx.Match(firstSuffix) || cnfFileExtRx.Match(ext) {
				return true
			}
			return false
		})
	}

	return nil
}

func (f *flare) collectHostSBOM(fb types.FlareBuilder) error {
	scanner := scanner.GetGlobalScanner()
	if scanner == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	scanRequest := host.NewScanRequest("/", os.DirFS("/"))
	scanResult := scanner.PerformScan(ctx, scanRequest, scanner.GetCollector(scanRequest.Collector()))
	if scanResult.Error != nil {
		return scanResult.Error
	}

	cycloneDX, err := scanResult.Report.ToCycloneDX()
	if err != nil {
		return err
	}

	jsonContent, err := json.MarshalIndent(cycloneDX, "", "  ")
	if err != nil {
		return err
	}

	return fb.AddFile("host-sbom.json", jsonContent)
}
