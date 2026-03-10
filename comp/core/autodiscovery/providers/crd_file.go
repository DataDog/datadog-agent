// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NameExtractorFunc extracts an integration check name from a config filename (without extension).
// It returns the check name and an error if the filename does not conform to the expected convention.
type NameExtractorFunc func(filenameWithoutExt string) (checkName string, err error)

// DefaultCRDNameExtractor returns the portion of the filename after the last '_'.
// e.g. "mynamespace_mypod_redis" → "redis"
func DefaultCRDNameExtractor(filenameWithoutExt string) (string, error) {
	idx := strings.LastIndex(filenameWithoutExt, "_")
	if idx < 0 || idx == len(filenameWithoutExt)-1 {
		return "", fmt.Errorf("filename %q does not match expected convention <NAMESPACE_NAME_CHECKNAME>", filenameWithoutExt)
	}
	return filenameWithoutExt[idx+1:], nil
}

// CRDFileConfigProvider collects check configurations from a directory populated
// by an external CRD controller and mounted into the agent container via a
// Kubernetes ConfigMap.
type CRDFileConfigProvider struct {
	dir            string
	nameExtractor  NameExtractorFunc
	Errors         map[string]string
	telemetryStore *telemetry.Store
}

// NewCRDFileConfigProvider creates a new CRDFileConfigProvider.
func NewCRDFileConfigProvider(dir string, extractor NameExtractorFunc, telemetryStore *telemetry.Store) *CRDFileConfigProvider {
	return &CRDFileConfigProvider{
		dir:            dir,
		nameExtractor:  extractor,
		Errors:         make(map[string]string),
		telemetryStore: telemetryStore,
	}
}

// Collect returns the check configurations found in the CRD config directory.
// Configs with advanced AD identifiers (kube_services, kube_endpoints CEL selectors) are filtered out.
func (c *CRDFileConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warnf("CRDFileConfigProvider: directory %q does not exist (expected mount point from ConfigMap)", c.dir)
		} else {
			log.Warnf("CRDFileConfigProvider: error reading directory %q: %s", c.dir, err)
		}
		return []integration.Config{}, nil
	}

	if len(entries) == 0 {
		log.Debugf("CRDFileConfigProvider: directory %q is empty (no CRD-driven checks configured)", c.dir)
		return []integration.Config{}, nil
	}

	integrationErrors := make(map[string]string)
	var configs []integration.Config

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		ext := filepath.Ext(fileName)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		filenameWithoutExt := strings.TrimSuffix(fileName, ext)
		checkName, err := c.nameExtractor(filenameWithoutExt)
		if err != nil {
			log.Warnf("CRDFileConfigProvider: skipping file %q: %s", fileName, err)
			integrationErrors[filenameWithoutExt] = err.Error()
			continue
		}

		absPath := filepath.Join(c.dir, fileName)
		conf, _, err := GetIntegrationConfigFromFile(checkName, absPath)
		if err != nil {
			log.Warnf("CRDFileConfigProvider: %q is not a valid config file: %s", absPath, err)
			integrationErrors[checkName] = err.Error()
			continue
		}

		if !WithoutAdvancedAD(conf) {
			log.Debugf("CRDFileConfigProvider: skipping config %q with advanced AD identifiers", checkName)
			continue
		}

		configs = append(configs, conf)
	}

	c.Errors = integrationErrors
	if c.telemetryStore != nil {
		c.telemetryStore.Errors.Set(float64(len(integrationErrors)), names.CRDFile)
	}

	return configs, nil
}

// IsUpToDate always returns false — polling is driven by the config poller.
func (c *CRDFileConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return false, nil
}

// String returns a string representation of the CRDFileConfigProvider.
func (c *CRDFileConfigProvider) String() string {
	return names.CRDFile
}

// GetConfigErrors returns the errors encountered when collecting configs.
func (c *CRDFileConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
