// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/lite"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// checker is the periodic built-in check. readFile is overridable so tests
// can simulate file-not-found / permission errors without touching disk.
type checker struct {
	cfg      config.Component
	readFile func(string) ([]byte, error)
}

func newChecker(cfg config.Component) *checker {
	return &checker{cfg: cfg, readFile: os.ReadFile}
}

// Run reads datadog.yaml from disk every tick (rather than the live merged
// config) so this check's output mirrors the rescue path's output exactly.
func (c *checker) Run() (*healthplatform.IssueReport, error) {
	path := c.configFilePath()
	if path == "" {
		return nil, nil
	}
	raw, err := c.readFile(path)
	if err != nil {
		// Missing file / permission denied are owned by other modules.
		return nil, nil
	}

	result := lite.ValidateRawConfig(raw)
	switch result.Verdict {
	case lite.VerdictOK:
		return nil, nil

	case lite.VerdictYAMLParseFailure:
		return &healthplatform.IssueReport{
			IssueId: healthplatformdef.InvalidConfigIssueID,
			Context: map[string]string{
				lite.ContextKeyErrorKind:    string(lite.ErrorKindYAMLParse),
				lite.ContextKeyConfigPath:   path,
				lite.ContextKeyErrorMessage: result.ParseError.Error(),
			},
			Tags: []string{"config", "yaml_parse"},
		}, nil

	case lite.VerdictSchemaInvalid:
		visible := result.SchemaErrors
		truncated := false
		if len(visible) > lite.MaxSchemaErrorsInPayload {
			visible = visible[:lite.MaxSchemaErrorsInPayload]
			truncated = true
		}
		return &healthplatform.IssueReport{
			IssueId: healthplatformdef.InvalidConfigIssueID,
			Context: map[string]string{
				lite.ContextKeyErrorKind:  string(lite.ErrorKindSchemaValidation),
				lite.ContextKeyConfigPath: path,
				lite.ContextKeyErrorCount: strconv.Itoa(len(result.SchemaErrors)),
				lite.ContextKeyErrors:     strings.Join(visible, "\n"),
				lite.ContextKeyTruncated:  strconv.FormatBool(truncated),
			},
			Tags: []string{"config", "schema"},
		}, nil

	case lite.VerdictSchemaUnavailable:
		// Build problem, not a customer problem — log and skip.
		pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check")
		return nil, nil
	}
	return nil, nil
}

func (c *checker) configFilePath() string {
	if c.cfg != nil {
		if p := c.cfg.ConfigFileUsed(); p != "" {
			return p
		}
	}
	return lite.DefaultConfigPath()
}
