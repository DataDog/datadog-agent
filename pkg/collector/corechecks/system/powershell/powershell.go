// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package powershell implements a Windows core check that runs admin-allowlisted,
// read-only PowerShell Get-* cmdlets and maps their output to metrics and tags.
package powershell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// CheckName is the name of the check
	CheckName = "powershell"

	// allowlistFileName is the fixed name of the admin-owned allowlist, always
	// read from the installer's admin-only <ProgramData>\Datadog\protected dir.
	allowlistFileName = "powershell_allowlist.yaml"

	// maxOutputBytes bounds the JSON captured from a single cmdlet invocation.
	maxOutputBytes = 10 * 1024 * 1024
)

// requiredEnvVars is the restricted environment passed to powershell.exe. It
// includes PSModulePath so server-role modules (FailoverClusters, PKI, ...)
// auto-load, plus the minimum Windows vars PowerShell needs to run.
var requiredEnvVars = []string{
	"SYSTEMROOT", "SYSTEMDRIVE", "COMSPEC", "PATH", "PATHEXT",
	"WINDIR", "TEMP", "TMP", "PSMODULEPATH", "PROGRAMFILES", "PROGRAMW6432",
}

// PowershellCheck runs one allowlisted Get-* cmdlet instance.
type PowershellCheck struct {
	core.CheckBase
	instance  *instanceConfig
	allowlist *allowlist
}

// Configure parses the instance config, loads and verifies the admin allowlist,
// and validates the instance against it. It fails closed on any problem.
func (c *PowershellCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	c.BuildID(integrationConfigDigest, data, initConfig)
	if err := c.CommonConfigure(senderManager, initConfig, data, source, provider); err != nil {
		return err
	}

	// Structural validation first, so malformed configs are logged clearly and
	// all schema errors are surfaced at once.
	if err := validateInstanceSchema(data); err != nil {
		return err
	}

	inst, err := parseInstanceConfig(data)
	if err != nil {
		log.Errorf("powershell check: invalid config: %s", err)
		return fmt.Errorf("invalid powershell check config: %w", err)
	}

	al, err := loadAllowlist()
	if err != nil {
		log.Errorf("powershell check: could not load allowlist: %s", err)
		return fmt.Errorf("could not load PowerShell allowlist: %w", err)
	}
	if err := al.validateInstance(inst); err != nil {
		log.Errorf("powershell check (cmdlet %q): rejected by allowlist: %s", inst.Cmdlet, err)
		return fmt.Errorf("instance rejected by allowlist: %w", err)
	}

	c.instance = inst
	c.allowlist = al

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()
	return nil
}

// Run invokes the cmdlet, maps its output to metrics/tags, and commits.
func (c *PowershellCheck) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}

	rows, err := c.runCmdlet(c.instance.Cmdlet, c.instance.Filters, c.instance.selectProperties())
	if err != nil {
		return fmt.Errorf("cmdlet %q failed: %w", c.instance.Cmdlet, err)
	}

	joins := c.runTagQueries()

	for _, row := range rows {
		tags := buildTags(c.instance, row, joins)
		for i := range c.instance.Metrics {
			if err := c.submitMetric(s, &c.instance.Metrics[i], row, tags); err != nil {
				return err
			}
		}
	}

	s.Commit()
	return nil
}

// runTagQueries resolves each tag_queries join into a map from link-target
// value to the joined target-property value. A failing join yields a nil map
// (its tags are simply omitted) rather than failing the whole run.
func (c *PowershellCheck) runTagQueries() []map[string]string {
	if len(c.instance.TagQueries) == 0 {
		return nil
	}
	joins := make([]map[string]string, len(c.instance.TagQueries))
	for i := range c.instance.TagQueries {
		q := &c.instance.TagQueries[i]
		rows, err := c.runCmdlet(q.TargetCmdlet, nil, []string{q.LinkTargetProperty, q.TargetProperty})
		if err != nil {
			log.Warnf("powershell check: tag_queries cmdlet %q failed, skipping join: %s", q.TargetCmdlet, err)
			continue
		}
		m := make(map[string]string, len(rows))
		for _, row := range rows {
			key := tagValue(row[q.LinkTargetProperty])
			val := tagValue(row[q.TargetProperty])
			if key != "" && val != "" {
				m[key] = val
			}
		}
		joins[i] = m
	}
	return joins
}

// submitMetric coerces a row property to a float and submits it under the
// configured metric type. Virtual (literal 1) metrics always submit 1.
//
// A non-virtual metric whose property is missing from the output or whose value
// is not numeric is a configuration error: there is no point running a check
// that cannot produce a metric value. It returns an error in that case so the
// whole run fails loudly (logged at error level and surfaced in `agent status`)
// rather than silently emitting nothing.
func (c *PowershellCheck) submitMetric(s sender.Sender, m *metricEntry, row map[string]interface{}, tags []string) error {
	var value float64
	if m.isVirtual() {
		value = 1
	} else {
		raw, ok := row[m.Property]
		if !ok {
			return fmt.Errorf("metric %q: property %q is not present in the output of cmdlet %q", m.Name, m.Property, c.instance.Cmdlet)
		}
		f, ok := toFloat(raw)
		if !ok {
			return fmt.Errorf("metric %q: property %q value %v is not numeric", m.Name, m.Property, raw)
		}
		value = f
	}

	name := c.instance.metricName(m)
	switch m.Type {
	case "rate":
		s.Rate(name, value, "", tags)
	case "count":
		s.Count(name, value, "", tags)
	case "monotonic_count":
		s.MonotonicCount(name, value, "", tags)
	case "histogram":
		s.Histogram(name, value, "", tags)
	case "distribution":
		s.Distribution(name, value, "", tags)
	default:
		s.Gauge(name, value, "", tags)
	}
	return nil
}

// runCmdlet builds the injection-safe command, spawns a one-shot powershell.exe
// under a timeout with a restricted environment, and parses the JSON output.
func (c *PowershellCheck) runCmdlet(cmdlet string, filters []filterEntry, selectProps []string) ([]map[string]interface{}, error) {
	script, err := buildCommand(cmdlet, filters, selectProps)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.instance.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", script)
	cmd.Env = restrictedEnv()

	stdout := &cappedBuffer{limit: maxOutputBytes}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timed out after %ds", c.instance.Timeout)
		}
		return nil, fmt.Errorf("%w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return parseRows(stdout.Bytes())
}

// parseRows decodes the compact JSON emitted by the dispatcher command into a
// list of rows. Empty output (no results) yields an empty slice. A single
// object (should not occur given the @() wrapper) is coerced into one row.
func parseRows(out []byte) ([]map[string]interface{}, error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(trimmed, &rows); err == nil {
		return rows, nil
	}

	var single map[string]interface{}
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return nil, fmt.Errorf("could not parse cmdlet JSON output: %w", err)
	}
	return []map[string]interface{}{single}, nil
}

// loadAllowlist resolves the fixed allowlist path, verifies it is owned by an
// administrator, reads it, and parses it. Any failure is fatal (fail closed).
func loadAllowlist() (*allowlist, error) {
	path, err := allowlistPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("allowlist not found at %s: %w", path, err)
	}
	if err := verifyAdminOwned(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read allowlist %s: %w", path, err)
	}
	return parseAllowlist(data)
}

// allowlistPath returns the fixed, non-configurable allowlist location inside
// the installer's admin-only protected directory.
func allowlistPath() (string, error) {
	base, err := winutil.GetProgramDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "protected", allowlistFileName), nil
}

// verifyAdminOwned fails unless the file is owned by the Administrators group or
// the SYSTEM account — the integrity guard that the allowlist was placed by an
// admin. (We check ownership rather than reuse filesystem.CheckRights because
// files under protected\ carry inherited ACEs that CheckRights would reject.)
func verifyAdminOwned(path string) error {
	var owner *windows.SID
	if err := winutil.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION,
		&owner, nil, nil, nil, nil); err != nil {
		return fmt.Errorf("could not read owner of %s: %w", path, err)
	}
	admins, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return err
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return err
	}
	if !windows.EqualSid(owner, admins) && !windows.EqualSid(owner, system) {
		return fmt.Errorf("allowlist %s must be owned by Administrators or SYSTEM", path)
	}
	return nil
}

// restrictedEnv returns a filtered copy of the process environment limited to
// requiredEnvVars.
func restrictedEnv() []string {
	allowed := make(map[string]struct{}, len(requiredEnvVars))
	for _, name := range requiredEnvVars {
		allowed[name] = struct{}{}
	}
	var env []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if _, ok := allowed[strings.ToUpper(parts[0])]; ok {
			env = append(env, e)
		}
	}
	return env
}

// cappedBuffer accumulates output up to a byte limit, discarding the rest so a
// runaway cmdlet cannot exhaust memory.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			c.buf.Write(p[:remaining])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else {
		c.truncated = true
	}
	// Always report a full write so the child process does not see a broken pipe.
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &PowershellCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
