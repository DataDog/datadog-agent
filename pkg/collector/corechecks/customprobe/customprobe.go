// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package customprobe implements customer-defined probes and optional local remediation.
package customprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check.
const CheckName = "custom_probe"

const maxProbeBodyBytes = 5 << 20

var errProbeBodyTooLarge = errors.New("probe response exceeded 5MiB")

type remediationConfig struct {
	Type           string   `yaml:"type"`
	Run            []string `yaml:"run"`
	URL            string   `yaml:"url"`
	Method         string   `yaml:"method"`
	Timeout        int      `yaml:"timeout"`
	MaxAttempts    int      `yaml:"max_attempts"`
	BackoffSeconds int      `yaml:"backoff_seconds"`
	DryRun         bool     `yaml:"dry_run"`
}

type instanceConfig struct {
	Name             string             `yaml:"name"`
	Type             string             `yaml:"type"`
	ServiceCheckName string             `yaml:"service_check_name"`
	Timeout          int                `yaml:"timeout"`
	Tags             []string           `yaml:"tags"`
	URL              string             `yaml:"url"`
	Method           string             `yaml:"method"`
	JSONPath         string             `yaml:"json_path"`
	WarningAbove     *float64           `yaml:"warning_above"`
	CriticalAbove    *float64           `yaml:"critical_above"`
	ContentMatch     string             `yaml:"content_match"`
	ExpectedStatus   int                `yaml:"expected_status"`
	Host             string             `yaml:"host"`
	Port             int                `yaml:"port"`
	Run              []string           `yaml:"run"`
	ExpectedExit     int                `yaml:"expected_exit"`
	StdoutMatch      string             `yaml:"stdout_match"`
	Path             string             `yaml:"path"`
	MustExist        bool               `yaml:"must_exist"`
	MaxAgeSeconds    int                `yaml:"max_age_seconds"`
	MinSizeBytes     *int64             `yaml:"min_size_bytes"`
	MaxSizeBytes     *int64             `yaml:"max_size_bytes"`
	Remediation      *remediationConfig `yaml:"remediation"`
}

type initConfig struct {
	EnableRemediation bool `yaml:"enable_remediation"`
}

type probeResult struct {
	status  servicecheck.ServiceCheckStatus
	message string
	err     error
}

// Check probes one configured target and retains its local remediation budget.
type Check struct {
	core.CheckBase
	cfg               instanceConfig
	client            *http.Client
	tags              []string
	enableRemediation bool
	contentMatch      *regexp.Regexp
	stdoutMatch       *regexp.Regexp
	attempts          int
	lastAttempt       time.Time
}

// Factory creates a new check factory.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{CheckBase: core.NewCheckBase(CheckName)}
}

// Configure initializes the instance and its sender.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	cfg := instanceConfig{Timeout: 5, Method: http.MethodGet, MustExist: true}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse custom_probe instance: %w", err)
	}
	var initCfg initConfig
	if err := yaml.Unmarshal(rawInitConfig, &initCfg); err != nil {
		return fmt.Errorf("parse custom_probe init_config: %w", err)
	}
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("parse custom_probe fields: %w", err)
	}
	if err := validateConfig(&cfg, fields); err != nil {
		return err
	}
	if (cfg.Type == "command" || cfg.Type == "file") && provider != names.File {
		return fmt.Errorf("%s probes require trusted local file configuration", cfg.Type)
	}
	var contentMatch, stdoutMatch *regexp.Regexp
	var err error
	if cfg.ContentMatch != "" {
		contentMatch, err = regexp.Compile(cfg.ContentMatch)
		if err != nil {
			return fmt.Errorf("invalid content_match: %w", err)
		}
	}
	if cfg.StdoutMatch != "" {
		stdoutMatch, err = regexp.Compile(cfg.StdoutMatch)
		if err != nil {
			return fmt.Errorf("invalid stdout_match: %w", err)
		}
	}
	if cfg.ServiceCheckName == "" {
		// Customer names may collide with other service checks; service_check_name can override them.
		cfg.ServiceCheckName = cfg.Name
	}
	host, err := hostname.Get(context.Background())
	if err != nil {
		return fmt.Errorf("resolve custom_probe hostname: %w", err)
	}
	c.BuildID(integrationConfigDigest, data, rawInitConfig)
	if err := c.CommonConfigure(senderManager, rawInitConfig, data, source, provider); err != nil {
		return err
	}
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()
	c.cfg = cfg
	c.client = timedClient(cfg.Timeout)
	// Annotation-provided init_config is not an operator-controlled execution gate.
	c.enableRemediation = initCfg.EnableRemediation && provider == names.File
	c.contentMatch, c.stdoutMatch = contentMatch, stdoutMatch
	c.attempts, c.lastAttempt = 0, time.Time{}
	c.tags = append(append([]string{}, cfg.Tags...), "probe_type:"+cfg.Type, "custom_probe:"+cfg.Name, "correlation_key:"+CheckName+":"+cfg.Name+":"+host)
	if cfg.Remediation != nil {
		c.tags = append(c.tags, "remediation_type:"+cfg.Remediation.Type)
	}
	return nil
}

func validateConfig(cfg *instanceConfig, fields map[string]yaml.Node) error {
	if err := validateProbeFields(cfg.Type, fields); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return errors.New("custom_probe requires name")
	}
	if !validSeconds(cfg.Timeout) {
		return errors.New("timeout must be positive seconds representable as a duration")
	}
	for _, threshold := range []*float64{cfg.WarningAbove, cfg.CriticalAbove} {
		if threshold != nil && (math.IsNaN(*threshold) || math.IsInf(*threshold, 0)) {
			return errors.New("warning_above and critical_above must be finite numbers")
		}
	}
	if cfg.WarningAbove != nil && cfg.CriticalAbove != nil && *cfg.WarningAbove > *cfg.CriticalAbove {
		return errors.New("warning_above must not exceed critical_above")
	}
	switch cfg.Type {
	case "http":
		if err := validateHTTP(cfg.URL, cfg.Method); err != nil {
			return err
		}
		if cfg.ExpectedStatus != 0 && (cfg.ExpectedStatus < 100 || cfg.ExpectedStatus > 599) {
			return errors.New("expected_status must be an HTTP status code")
		}
		if cfg.JSONPath == "" && (cfg.WarningAbove != nil || cfg.CriticalAbove != nil) {
			return errors.New("HTTP thresholds require json_path")
		}
	case "tcp":
		if strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 || cfg.Port > 65535 {
			return errors.New("TCP probe requires host and port between 1 and 65535")
		}
	case "command":
		if err := validateCommand(cfg.Run); err != nil {
			return err
		}
		if cfg.ExpectedExit < 0 || cfg.ExpectedExit > 255 {
			return errors.New("expected_exit must be between 0 and 255")
		}
	case "file":
		if strings.TrimSpace(cfg.Path) == "" {
			return errors.New("file probe requires path")
		}
		if cfg.MaxAgeSeconds < 0 || (cfg.MaxAgeSeconds > 0 && !validSeconds(cfg.MaxAgeSeconds)) {
			return errors.New("max_age_seconds must be nonnegative seconds representable as a duration")
		}
		if (cfg.MinSizeBytes != nil && *cfg.MinSizeBytes < 0) || (cfg.MaxSizeBytes != nil && *cfg.MaxSizeBytes < 0) {
			return errors.New("file size limits must be nonnegative")
		}
		if cfg.MinSizeBytes != nil && cfg.MaxSizeBytes != nil && *cfg.MinSizeBytes > *cfg.MaxSizeBytes {
			return errors.New("min_size_bytes must not exceed max_size_bytes")
		}
	default:
		return errors.New("custom_probe requires type http, tcp, command, or file")
	}
	if cfg.Remediation != nil {
		r := cfg.Remediation
		var remediationFields map[string]yaml.Node
		node := fields["remediation"]
		if err := node.Decode(&remediationFields); err != nil {
			return fmt.Errorf("parse remediation fields: %w", err)
		}
		for _, field := range []string{"run", "url", "method"} {
			_, present := remediationFields[field]
			if present && ((field == "run" && r.Type != "command") || (field != "run" && r.Type != "http")) {
				return fmt.Errorf("remediation field %s does not apply to type %s", field, r.Type)
			}
		}
		if r.Timeout == 0 {
			r.Timeout = 5
		}
		if r.MaxAttempts == 0 {
			r.MaxAttempts = 1
		}
		if r.BackoffSeconds == 0 {
			r.BackoffSeconds = 60
		}
		if !validSeconds(r.Timeout) || !validSeconds(r.BackoffSeconds) || r.MaxAttempts < 1 {
			return errors.New("remediation requires positive timeout, max_attempts, and backoff_seconds")
		}
		switch r.Type {
		case "command":
			if err := validateCommand(r.Run); err != nil {
				return fmt.Errorf("remediation: %w", err)
			}
		case "http":
			if r.Method == "" {
				r.Method = http.MethodPost
			}
			if err := validateHTTP(r.URL, r.Method); err != nil {
				return fmt.Errorf("remediation: %w", err)
			}
		default:
			return errors.New("remediation requires type command or http")
		}
	}
	return nil
}

func validateProbeFields(probeType string, fields map[string]yaml.Node) error {
	for _, group := range []struct {
		types  []string
		fields []string
	}{
		{[]string{"http"}, []string{"url", "method", "json_path", "warning_above", "critical_above", "expected_status"}},
		{[]string{"tcp"}, []string{"host", "port"}},
		{[]string{"command"}, []string{"run", "expected_exit", "stdout_match"}},
		{[]string{"file"}, []string{"path", "must_exist", "max_age_seconds", "min_size_bytes", "max_size_bytes"}},
		{[]string{"http", "file"}, []string{"content_match"}},
	} {
		if slices.Contains(group.types, probeType) {
			continue
		}
		for _, field := range group.fields {
			if _, present := fields[field]; present {
				return fmt.Errorf("field %s does not apply to probe type %s", field, probeType)
			}
		}
	}
	return nil
}

func validSeconds(seconds int) bool {
	return seconds > 0 && int64(seconds) <= math.MaxInt64/int64(time.Second)
}

func validateHTTP(rawURL, method string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid HTTP URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("url must be an HTTP(S) URL with a host")
	}
	if _, err := http.NewRequest(method, rawURL, nil); err != nil {
		return fmt.Errorf("invalid HTTP request: %w", sanitizeHTTPError(err))
	}
	return nil
}

func timedClient(timeout int) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Connect directly so a proxy cannot bypass target IP validation.
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: time.Duration(timeout) * time.Second, Control: controlProbeConnection}
	transport.DialContext = dialer.DialContext
	return &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func controlProbeConnection(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	// Strip the zone from scoped IPv6 addresses before parsing the resolved IP.
	host, _, _ = strings.Cut(host, "%")
	ip := net.ParseIP(host)
	if ip == nil {
		return errors.New("invalid resolved target IP")
	}
	// Broader private-IP SSRF from annotations is an accepted POC limitation; workload access needs future PAR/authz controls.
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return errors.New("link-local/metadata targets are blocked")
	}
	return nil
}

func sanitizeHTTPError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	safeURL := "<invalid URL>"
	if u, parseErr := url.Parse(urlErr.URL); parseErr == nil {
		u.User, u.RawQuery, u.Fragment, u.RawFragment = nil, "", "", ""
		u.ForceQuery = false
		safeURL = u.String()
	}
	return &url.Error{Op: urlErr.Op, URL: safeURL, Err: sanitizeHTTPError(urlErr.Err)}
}

// Run detects, optionally remediates, verifies, and emits exactly one event and service check.
func (c *Check) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	result := c.probe()
	outcome := "unknown"
	switch result.status {
	case servicecheck.ServiceCheckOK:
		outcome = "passing"
	case servicecheck.ServiceCheckWarning:
		outcome = "warning"
	case servicecheck.ServiceCheckCritical:
		outcome = "detected"
	}
	title := outcome + ": " + c.cfg.Name
	detail := ""
	if r := c.cfg.Remediation; result.status == servicecheck.ServiceCheckCritical && r != nil {
		if time.Since(c.lastAttempt) >= time.Duration(r.BackoffSeconds)*time.Second {
			c.attempts = 0
		}
		if c.attempts >= r.MaxAttempts {
			detail = "remediation attempt budget exhausted within backoff window"
		} else if !c.enableRemediation || r.DryRun {
			title = "dry-run: would run " + r.Type + " remediation for " + c.cfg.Name
		} else {
			c.attempts++
			c.lastAttempt = time.Now()
			if err := c.remediate(); err != nil {
				detail = "remediation failed: " + err.Error()
			}
			result = c.probe()
			if result.status == servicecheck.ServiceCheckOK || result.status == servicecheck.ServiceCheckWarning {
				title = "remediated: " + c.cfg.Name + " recovered"
			} else {
				title = "escalate: remediation did not recover " + c.cfg.Name
			}
		}
	}
	if detail != "" {
		result.message += "; " + detail
	}
	alertType := event.AlertTypeInfo
	switch result.status {
	case servicecheck.ServiceCheckOK:
		alertType = event.AlertTypeSuccess
	case servicecheck.ServiceCheckWarning:
		alertType = event.AlertTypeWarning
	case servicecheck.ServiceCheckCritical, servicecheck.ServiceCheckUnknown:
		alertType = event.AlertTypeError
	}
	s.Event(event.Event{Title: title, Text: result.message, Ts: time.Now().Unix(), Tags: c.tags, AlertType: alertType, SourceTypeName: CheckName})
	s.ServiceCheck(c.cfg.ServiceCheckName, result.status, "", c.tags, result.message)
	s.Commit()
	return result.err
}

func unknown(err error) probeResult {
	return probeResult{status: servicecheck.ServiceCheckUnknown, message: err.Error(), err: err}
}

func (c *Check) probe() probeResult {
	switch c.cfg.Type {
	case "http":
		return c.probeHTTP()
	case "tcp":
		return c.probeTCP()
	case "command":
		return c.probeCommand()
	case "file":
		return c.probeFile()
	default:
		return unknown(errors.New("unsupported probe type"))
	}
}

func (c *Check) probeHTTP() probeResult {
	request, err := http.NewRequest(c.cfg.Method, c.cfg.URL, nil)
	if err != nil {
		return unknown(fmt.Errorf("create HTTP probe: %w", sanitizeHTTPError(err)))
	}
	response, err := c.client.Do(request)
	if err != nil {
		return probeResult{status: servicecheck.ServiceCheckCritical, message: fmt.Sprintf("HTTP probe failed: %v", sanitizeHTTPError(err))}
	}
	defer response.Body.Close()
	result := probeResult{status: servicecheck.ServiceCheckOK, message: "HTTP status " + response.Status}
	if c.cfg.ExpectedStatus != 0 {
		if response.StatusCode != c.cfg.ExpectedStatus {
			result.status = servicecheck.ServiceCheckCritical
			result.message += fmt.Sprintf("; expected_status=%d", c.cfg.ExpectedStatus)
			return result
		}
	} else if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		result.status = servicecheck.ServiceCheckCritical
		return result
	}
	body, err := readProbeBody(response.Body)
	if err != nil {
		if !errors.Is(err, errProbeBodyTooLarge) {
			return probeResult{status: servicecheck.ServiceCheckCritical, message: fmt.Sprintf("HTTP probe failed: %v", sanitizeHTTPError(err))}
		}
		return unknown(sanitizeHTTPError(err))
	}
	if c.cfg.JSONPath != "" {
		var document interface{}
		if err := json.Unmarshal(body, &document); err != nil {
			return unknown(errors.New("probe response was not valid JSON"))
		}
		value, err := extractValue(document, c.cfg.JSONPath)
		if err != nil {
			return unknown(err)
		}
		if c.cfg.CriticalAbove != nil && value >= *c.cfg.CriticalAbove {
			result.status = servicecheck.ServiceCheckCritical
		} else if c.cfg.WarningAbove != nil && value >= *c.cfg.WarningAbove {
			result.status = servicecheck.ServiceCheckWarning
		}
		result.message += fmt.Sprintf("; %s=%g; warning_above=%s; critical_above=%s", c.cfg.JSONPath, value, formatThreshold(c.cfg.WarningAbove), formatThreshold(c.cfg.CriticalAbove))
	}
	if c.contentMatch != nil {
		matched := c.contentMatch.Match(body)
		result.message += fmt.Sprintf("; content_match=%t", matched)
		if !matched {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	return result
}

func readProbeBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxProbeBodyBytes+1))
	if len(body) > maxProbeBodyBytes {
		return nil, errProbeBodyTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("read probe response: %w", err)
	}
	return body, nil
}

func (c *Check) probeTCP() probeResult {
	address := net.JoinHostPort(c.cfg.Host, strconv.Itoa(c.cfg.Port))
	dialer := &net.Dialer{Timeout: time.Duration(c.cfg.Timeout) * time.Second, Control: controlProbeConnection}
	conn, err := dialer.DialContext(context.Background(), "tcp", address)
	if err != nil {
		return probeResult{status: servicecheck.ServiceCheckCritical, message: fmt.Sprintf("TCP connect to %s failed: %v", address, err)}
	}
	conn.Close()
	return probeResult{status: servicecheck.ServiceCheckOK, message: "TCP connected to " + address}
}

func validateCommand(run []string) error {
	if len(run) == 0 || strings.TrimSpace(run[0]) == "" {
		return errors.New("command requires run as a non-empty argv list")
	}
	return nil
}

type cappedOutput struct {
	data      []byte
	truncated bool
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > maxProbeBodyBytes-len(b.data) {
		b.truncated = true
	}
	if available := maxProbeBodyBytes - len(b.data); available > 0 {
		b.data = append(b.data, p[:min(len(p), available)]...)
	}
	return len(p), nil
}

func runCommand(run []string, timeout int) ([]byte, int, error) {
	if err := validateCommand(run); err != nil {
		return nil, -1, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, run[0], run[1:]...)
	configureCommandCancellation(cmd)
	var stdout cappedOutput
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return nil, -1, fmt.Errorf("start command: %w", err)
	}
	err := cmd.Wait()
	if ctx.Err() != nil {
		return stdout.data, -1, ctx.Err()
	}
	if stdout.truncated {
		return nil, -1, errors.New("command stdout exceeded 5MiB")
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return stdout.data, -1, err
	}
	return stdout.data, cmd.ProcessState.ExitCode(), nil
}

func (c *Check) probeCommand() probeResult {
	stdout, exitCode, err := runCommand(c.cfg.Run, c.cfg.Timeout)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return probeResult{status: servicecheck.ServiceCheckCritical, message: "command timed out"}
		}
		return unknown(err)
	}
	result := probeResult{status: servicecheck.ServiceCheckOK, message: fmt.Sprintf("command exit=%d; expected_exit=%d", exitCode, c.cfg.ExpectedExit)}
	if exitCode != c.cfg.ExpectedExit {
		result.status = servicecheck.ServiceCheckCritical
	}
	if c.stdoutMatch != nil {
		matched := c.stdoutMatch.Match(stdout)
		result.message += fmt.Sprintf("; stdout_match=%t", matched)
		if !matched {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	return result
}

func (c *Check) probeFile() probeResult {
	info, err := os.Stat(c.cfg.Path)
	if errors.Is(err, os.ErrNotExist) {
		if c.cfg.MustExist {
			return probeResult{status: servicecheck.ServiceCheckCritical, message: "required file is missing: " + c.cfg.Path}
		}
		return probeResult{status: servicecheck.ServiceCheckOK, message: "file is absent as required: " + c.cfg.Path}
	}
	if err != nil {
		return unknown(fmt.Errorf("stat file: %w", err))
	}
	if !c.cfg.MustExist {
		return probeResult{status: servicecheck.ServiceCheckCritical, message: "file exists but must_exist is false: " + c.cfg.Path}
	}
	age := time.Since(info.ModTime())
	result := probeResult{status: servicecheck.ServiceCheckOK, message: fmt.Sprintf("file %s exists; size=%d bytes; age=%.0f seconds", c.cfg.Path, info.Size(), age.Seconds())}
	if c.cfg.MaxAgeSeconds > 0 {
		result.message += fmt.Sprintf("; max_age_seconds=%d", c.cfg.MaxAgeSeconds)
		if age > time.Duration(c.cfg.MaxAgeSeconds)*time.Second {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	if c.cfg.MinSizeBytes != nil {
		result.message += fmt.Sprintf("; min_size_bytes=%d", *c.cfg.MinSizeBytes)
		if info.Size() < *c.cfg.MinSizeBytes {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	if c.cfg.MaxSizeBytes != nil {
		result.message += fmt.Sprintf("; max_size_bytes=%d", *c.cfg.MaxSizeBytes)
		if info.Size() > *c.cfg.MaxSizeBytes {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	if c.contentMatch != nil {
		if !info.Mode().IsRegular() {
			return unknown(errors.New("content_match requires a regular file"))
		}
		file, err := os.Open(c.cfg.Path)
		if err != nil {
			return unknown(fmt.Errorf("open file: %w", err))
		}
		defer file.Close()
		body, err := readProbeBody(file)
		if err != nil {
			return unknown(err)
		}
		matched := c.contentMatch.Match(body)
		result.message += fmt.Sprintf("; content_match=%t", matched)
		if !matched {
			result.status = servicecheck.ServiceCheckCritical
		}
	}
	return result
}

func (c *Check) remediate() error {
	r := c.cfg.Remediation
	if r.Type == "command" {
		// POC-local execution is intentionally unsandboxed; PAR/rshell sandboxing is a future extension.
		_, exitCode, err := runCommand(r.Run, r.Timeout)
		if err != nil {
			return err
		}
		if exitCode != 0 {
			return fmt.Errorf("command exited with code %d", exitCode)
		}
		return nil
	}
	request, err := http.NewRequest(r.Method, r.URL, nil)
	if err != nil {
		return sanitizeHTTPError(err)
	}
	client := timedClient(r.Timeout)
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return sanitizeHTTPError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP remediation returned status %s", response.Status)
	}
	return nil
}

func formatThreshold(value *float64) string {
	if value == nil {
		return "unset"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func extractValue(document interface{}, path string) (float64, error) {
	current := document
	var parts []string
	if path != "." && path != "$" {
		normalized := strings.TrimPrefix(strings.TrimPrefix(path, "$"), ".")
		normalized = strings.ReplaceAll(normalized, "[", ".")
		normalized = strings.ReplaceAll(normalized, "]", "")
		normalized = strings.TrimPrefix(normalized, ".")
		parts = strings.Split(normalized, ".")
	}
	for _, part := range parts {
		switch node := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = node[part]
			if !ok {
				return 0, fmt.Errorf("json_path %q: missing field %q", path, part)
			}
		case []interface{}:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return 0, fmt.Errorf("json_path %q: invalid array index %q", path, part)
			}
			current = node[index]
		default:
			return 0, fmt.Errorf("json_path %q: cannot traverse %q", path, part)
		}
	}
	value, ok := current.(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("json_path %q: value is not numeric", path)
	}
	return value, nil
}
