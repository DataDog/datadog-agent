// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml problems through the Agent Health Platform.
package invalidconfig

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/lite"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// checker is the periodic built-in check. Caches the last verdict against the
// file's modified time so an unchanged datadog.yaml isn't reparsed every interval
type checker struct {
	cfg               config.Component
	lastModified      time.Time
	lastReport        *healthplatform.IssueReport
	schemaUnavailable sync.Once
}

func newChecker(cfg config.Component) *checker {
	return &checker{cfg: cfg}
}

func (c *checker) Run() (*healthplatform.IssueReport, error) {
	path := c.cfg.ConfigFileUsed()
	if path == "" {
		return nil, nil
	}
	stat, err := os.Stat(path)
	if err != nil {
		// Missing file / permission denied are owned by other modules.
		return nil, nil
	}
	if stat.ModTime().Equal(c.lastModified) {
		return c.lastReport, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	info, raise := c.issueInfoFor(path, lite.ValidateRawConfig(raw))
	c.lastModified = stat.ModTime()
	if !raise {
		c.lastReport = nil
		return nil, nil
	}
	c.lastReport = &healthplatform.IssueReport{
		IssueId: healthplatformdef.InvalidConfigIssueID,
		Context: info.ToContext(),
		Tags:    info.Tags(),
	}
	return c.lastReport, nil
}

// issueInfoFor translates a validation verdict into the IssueInfo the platform
// expands via the template. Returns false when there is nothing to raise
// (healthy config or schema-validator infrastructure error).
func (c *checker) issueInfoFor(path string, result lite.ValidationResult) (lite.IssueInfo, bool) {
	switch result.Verdict {
	case lite.VerdictYAMLParseFailure:
		return lite.IssueInfo{
			Kind:         lite.ErrorKindYAMLParse,
			ConfigPath:   path,
			ErrorMessage: result.ParseError.Error(),
		}, true

	case lite.VerdictSchemaInvalid:
		visible, truncated := lite.TruncateSchemaErrors(result.SchemaErrors)
		return lite.IssueInfo{
			Kind:       lite.ErrorKindSchemaValidation,
			ConfigPath: path,
			Errors:     strings.Join(visible, "\n"),
			ErrorCount: len(result.SchemaErrors),
			Truncated:  truncated,
		}, true

	case lite.VerdictSchemaUnavailable:
		// log once to avoid spam
		c.schemaUnavailable.Do(func() {
			pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check")
		})
	}
	return lite.IssueInfo{}, false
}
