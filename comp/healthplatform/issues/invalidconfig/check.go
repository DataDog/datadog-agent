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

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/lite"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// checker is the periodic built-in check. Validates the on-disk datadog.yaml
// against the schema each interval
type checker struct {
	cfg               config.Component
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
	raw, err := os.ReadFile(path)
	if err != nil {
		// Missing file / permission denied are owned by other modules.
		return nil, nil
	}
	info, raise := c.issueInfoFor(path, lite.ValidateRawConfig(raw))
	if !raise {
		return nil, nil
	}
	return &healthplatform.IssueReport{
		IssueId: healthplatformdef.InvalidConfigIssueID,
		Context: info.ToContext(),
		Tags:    info.Tags(),
	}, nil
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
			pkglog.Warnf("[AGENTLITECONFIG] invalidconfig: schema validator unavailable; skipping check")
		})
	}
	return lite.IssueInfo{}, false
}
