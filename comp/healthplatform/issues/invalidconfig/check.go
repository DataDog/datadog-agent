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

// checker captures the dependencies a built-in check needs at run time. The
// module stamps a *checker into the BuiltInCheck and uses its Run method as
// the HealthCheckFunc.
type checker struct {
	cfg config.Component

	// readFile is overridable for tests; production callers always use
	// os.ReadFile. Wrapping it lets us simulate file-not-found / permission
	// errors without touching the real filesystem.
	readFile func(string) ([]byte, error)
}

// newChecker is the production constructor.
func newChecker(cfg config.Component) *checker {
	return &checker{cfg: cfg, readFile: os.ReadFile}
}

// Run is the HealthCheckFunc signature. It reads the configured datadog.yaml
// from disk, runs the lite validator, and returns the appropriate IssueReport
// (or nil to auto-clear any prior issue).
//
// Reading from disk on every tick — instead of inspecting the live merged
// config — keeps the in-Fx check's output identical to the rescue path's
// output and matches the customer's mental model: "I edit datadog.yaml, the
// agent tells me what's wrong with that file."
func (c *checker) Run() (*healthplatform.IssueReport, error) {
	path := c.configFilePath()
	if path == "" {
		// No config file means the agent is running on env vars only —
		// nothing for the schema validator to look at.
		return nil, nil
	}

	raw, err := c.readFile(path)
	if err != nil {
		// Not our problem (missing file, permissions). Other modules cover
		// permission issues directly.
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
				contextKeyErrorKind:    errorKindYAMLParse,
				contextKeyConfigPath:   path,
				contextKeyErrorMessage: result.ParseError.Error(),
			},
			Tags: []string{"config", "yaml_parse"},
		}, nil

	case lite.VerdictSchemaInvalid:
		visible := result.SchemaErrors
		truncated := false
		if len(visible) > maxErrorsInPayload {
			visible = visible[:maxErrorsInPayload]
			truncated = true
		}
		return &healthplatform.IssueReport{
			IssueId: healthplatformdef.InvalidConfigIssueID,
			Context: map[string]string{
				contextKeyErrorKind:  errorKindSchemaValidation,
				contextKeyConfigPath: path,
				contextKeyErrorCount: strconv.Itoa(len(result.SchemaErrors)),
				contextKeyErrors:     strings.Join(visible, "\n"),
				contextKeyTruncated:  boolStr(truncated),
			},
			Tags: []string{"config", "schema"},
		}, nil

	case lite.VerdictSchemaUnavailable:
		// The validator itself broke (embedded schema missing, decompress
		// failure). That's an agent-build problem, not a customer
		// problem; log once per tick and don't raise an issue.
		pkglog.Warnf("invalidconfig: schema validator unavailable; skipping check")
		return nil, nil
	}
	return nil, nil
}

// configFilePath returns the on-disk path of the resolved datadog.yaml.
// Falls back to the platform default if the config component cannot tell
// us (e.g. in test contexts).
func (c *checker) configFilePath() string {
	if c.cfg != nil {
		if p := c.cfg.ConfigFileUsed(); p != "" {
			return p
		}
	}
	return lite.DefaultConfigPath()
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
