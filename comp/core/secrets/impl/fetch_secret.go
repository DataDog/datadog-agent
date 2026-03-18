// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	json "github.com/json-iterator/go"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const secretsManagementDocsURL = "https://docs.datadoghq.com/agent/configuration/secrets-management"

type limitBuffer struct {
	max int
	buf *bytes.Buffer
}

func (b *limitBuffer) Write(p []byte) (n int, err error) {
	if len(p)+b.buf.Len() > b.max {
		return 0, fmt.Errorf("command output was too long: exceeded %d bytes", b.max)
	}
	return b.buf.Write(p)
}

func (r *secretResolver) execCommand(inputPayload string, timeout int) ([]byte, error) {
	// hook used only for tests
	if r.commandHookFunc != nil {
		return r.commandHookFunc(inputPayload)
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeout)*time.Second)
	defer cancel()

	cmd, done, err := commandContext(ctx, r.backendCommand, r.backendArguments...)
	if err != nil {
		return nil, err
	}
	defer done()

	if !r.embeddedBackendPermissiveRights {
		if err := checkRightsFunc(cmd.Path, r.commandAllowGroupExec); err != nil {
			return nil, err
		}
	}

	cmd.Stdin = strings.NewReader(inputPayload)

	stdout := limitBuffer{
		buf: &bytes.Buffer{},
		max: r.responseMaxSize,
	}
	stderr := limitBuffer{
		buf: &bytes.Buffer{},
		max: r.responseMaxSize,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// We add the actual time to the log message. This is needed in the case we have a secret in the datadog.yaml.
	// When it's the case the log package is not yet initialized (since it needs the configuration) and it will
	// buffer logs until it's initialized. This means the time of the log line will be the one after the package is
	// initialized and not the creation time. This is an issue when troubleshooting a secret_backend_command in
	// datadog.yaml.
	log.Debugf("%s | calling secret_backend_command with payload: '%s'", time.Now().String(), inputPayload)
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	log.Debugf("%s | secret_backend_command '%s' completed in %s", time.Now().String(), r.backendCommand, elapsed)

	// We always log stderr to allow a secret_backend_command to logs info in the agent log file. This is useful to
	// troubleshoot secret_backend_command in a containerized environment.
	if err != nil {
		log.Errorf("secret_backend_command stderr: %s", stderr.buf.String())

		exitCode := "unknown"
		var e *exec.ExitError
		if errors.As(err, &e) {
			exitCode = strconv.Itoa(e.ExitCode())
		} else if ctx.Err() == context.DeadlineExceeded {
			exitCode = "timeout"
		}
		r.tlmSecretBackendElapsed.Add(float64(elapsed.Milliseconds()), r.backendCommand, exitCode)

		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("'%s' timed out after %d seconds. You can increase secret_backend_timeout in datadog.yaml. Docs: %s",
				r.backendCommand, r.backendTimeout, secretsManagementDocsURL)
		}
		errStr := strings.ToLower(err.Error())
		stderrStr := stderr.buf.String()
		if strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "operation not permitted") || strings.Contains(errStr, "access is denied") {
			log.Warnf("'%s' failed: permission denied. See docs for more information on the setup secrets: %s",
				r.backendCommand, secretsManagementDocsURL)
			return nil, fmt.Errorf("permission denied executing secret command '%s'", r.backendCommand)
		}
		if strings.Contains(stderrStr, "invalid version") || strings.Contains(stderrStr, "expected 1.0") {
			log.Warnf("'%s' seems to have detected an invalid version. The Agent sends payload with version '%s'. If your script only works with version '1.0', update it. Docs: %s",
				r.backendCommand, secrets.PayloadVersion, secretsManagementDocsURL)
		} else {
			log.Warnf("'%s' failed (exit code %s, message: '%s'). See docs for FAQ and troubleshooting methods: %s",
				r.backendCommand, exitCode, err, secretsManagementDocsURL)
		}
		return nil, fmt.Errorf("error while running '%s': %s. See docs for FAQ and troubleshooting methods: %s", r.backendCommand, err, secretsManagementDocsURL)
	}

	log.Debugf("secret_backend_command stderr: %s", stderr.buf.String())

	r.tlmSecretBackendElapsed.Add(float64(elapsed.Milliseconds()), r.backendCommand, "0")
	return stdout.buf.Bytes(), nil
}

func (r *secretResolver) fetchSecretBackendVersion() (string, error) {
	// hook used only for tests
	if r.versionHookFunc != nil {
		return r.versionHookFunc()
	}

	// Only get version when secret_backend_type or extra_secret_backends is used
	if r.backendType == "" && len(r.backendConfigs) == 0 {
		return "", errors.New("version only supported when secret_backend_type or extra_secret_backends is configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		min(time.Duration(r.backendTimeout)*time.Second, 1*time.Second))
	defer cancel()

	// Execute with --version argument
	cmd, done, err := commandContext(ctx, r.backendCommand, "--version")
	if err != nil {
		return "", err
	}
	defer done()

	if !r.embeddedBackendPermissiveRights {
		if err := filesystem.CheckRights(cmd.Path, r.commandAllowGroupExec); err != nil {
			return "", err
		}
	}

	stdout := limitBuffer{
		buf: &bytes.Buffer{},
		max: r.responseMaxSize,
	}
	stderr := limitBuffer{
		buf: &bytes.Buffer{},
		max: r.responseMaxSize,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Debugf("calling secret_backend_command --version")
	err = cmd.Run()

	if err != nil {
		log.Debugf("secret_backend_command --version stderr: %s", stderr.buf.String())
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("version command timeout")
		}
		return "", fmt.Errorf("version command failed: %w", err)
	}

	return strings.TrimSpace(stdout.buf.String()), nil
}

// splitSecretHandle splits a handle on "::" returning (backendID, secretKey).
// If no "::" is present, backendID is "" (default backend) and the full string is the key.
// The double-colon delimiter avoids ambiguity with handle formats that already contain
// a single colon, such as vault://path#/json/pointer or Windows absolute paths (C:\...).
func splitSecretHandle(handle string) (backendID, secretKey string) {
	const delim = "::"
	idx := strings.Index(handle, delim)
	if idx == -1 {
		return "", handle
	}
	return handle[:idx], handle[idx+len(delim):]
}

// resolveBackendConfig returns the type, config, and timeout for a named backend.
// An empty backendID or the literal "default" refers to the default backend
// (secret_backend_type / secret_backend_config), so ENC[default::key] and ENC[key]
// are equivalent. The timeout falls back to the global r.backendTimeout if not set.
func (r *secretResolver) resolveBackendConfig(backendID string) (string, map[string]interface{}, int, error) {
	if backendID == "" || backendID == "default" {
		return r.backendType, r.backendConfig, r.backendTimeout, nil
	}
	raw, ok := r.backendConfigs[backendID]
	if !ok {
		return "", nil, 0, fmt.Errorf("unknown backend %q", backendID)
	}
	entry, ok := raw.(map[string]interface{})
	if !ok {
		return "", nil, 0, fmt.Errorf("invalid config for backend %q", backendID)
	}
	bType, _ := entry["type"].(string)
	bConfig, _ := entry["config"].(map[string]interface{})
	if bConfig == nil {
		bConfig = make(map[string]interface{})
	}
	bTimeout := r.backendTimeout
	switch v := entry["secret_backend_timeout"].(type) {
	case int:
		bTimeout = v
	case float64:
		bTimeout = int(v)
	}
	return bType, bConfig, bTimeout, nil
}

// fetchSecret groups the provided handles by backend (using the "::" delimiter),
// calls fetchSingleBackend once per backend, and returns a merged result keyed by
// the original handles. Each backend is attempted independently so a failure in one
// does not affect the others. Per-handle errors are returned in the second map.
func (r *secretResolver) fetchSecret(handles []string) (map[string]string, map[string]error) {
	type group struct {
		backendType    string
		backendConfig  map[string]interface{}
		backendTimeout int
		keys           []string // stripped secret keys sent to the binary
		origHandles    []string // original handles for result remapping
		cfgErr         error    // set when the backend ID could not be resolved
	}

	groups := map[string]*group{}
	for _, handle := range handles {
		backendID, secretKey := splitSecretHandle(handle)
		// Normalize "default" to "" so ENC[default::key] and ENC[key] share the same group
		// and result in a single backend call.
		if backendID == "default" {
			backendID = ""
		}
		if _, exists := groups[backendID]; !exists {
			bType, bConfig, bTimeout, err := r.resolveBackendConfig(backendID)
			groups[backendID] = &group{backendType: bType, backendConfig: bConfig, backendTimeout: bTimeout, cfgErr: err}
		}
		groups[backendID].keys = append(groups[backendID].keys, secretKey)
		groups[backendID].origHandles = append(groups[backendID].origHandles, handle)
	}

	result := make(map[string]string, len(handles))
	var handleErrors map[string]error
	for _, g := range groups {
		if g.cfgErr != nil {
			if handleErrors == nil {
				handleErrors = make(map[string]error)
			}
			for _, h := range g.origHandles {
				handleErrors[h] = fmt.Errorf("handle %q: %s", h, g.cfgErr)
			}
			continue
		}
		res, perHandleErrs, globalErr := r.fetchSingleBackend(g.backendType, g.backendConfig, g.backendTimeout, g.keys)
		if globalErr != nil {
			if handleErrors == nil {
				handleErrors = make(map[string]error)
			}
			for _, h := range g.origHandles {
				handleErrors[h] = globalErr
			}
			continue
		}
		for i, key := range g.keys {
			if val, ok := res[key]; ok {
				result[g.origHandles[i]] = val
			} else if err, ok := perHandleErrs[key]; ok {
				if handleErrors == nil {
					handleErrors = make(map[string]error)
				}
				handleErrors[g.origHandles[i]] = err
			}
		}
	}
	return result, handleErrors
}

// fetchSingleBackend calls the secret backend command for a single backend type/config.
// It returns:
//   - resolved: map of secret key → resolved value for handles that succeeded
//   - handleErrors: map of secret key → error for handles that failed individually
//   - err: a global/fatal error (command failure, JSON unmarshal) that affects all handles
func (r *secretResolver) fetchSingleBackend(backendType string, backendConfig map[string]interface{}, backendTimeout int, secretsHandle []string) (resolved map[string]string, handleErrors map[string]error, err error) {
	payload := map[string]interface{}{
		"version":                secrets.PayloadVersion,
		"secrets":                secretsHandle,
		"secret_backend_timeout": backendTimeout,
	}
	if backendType != "" {
		payload["type"] = backendType
	}
	if len(backendConfig) > 0 {
		payload["config"] = backendConfig
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("could not serialize secrets IDs to fetch password: %s", err)
	}
	output, err := r.execCommand(string(jsonPayload), backendTimeout)
	if err != nil {
		return nil, nil, err
	}

	secretVals := map[string]secrets.SecretVal{}
	if err = json.Unmarshal(output, &secretVals); err != nil {
		r.tlmSecretUnmarshalError.Inc()
		return nil, nil, fmt.Errorf("'%s' returned invalid JSON: '%s'. See docs for expected format: %s",
			r.backendCommand, err, secretsManagementDocsURL)
	}

	resolved = map[string]string{}
	for _, sec := range secretsHandle {
		v, ok := secretVals[sec]
		if !ok {
			r.tlmSecretResolveError.Inc("missing", sec)
			if handleErrors == nil {
				handleErrors = make(map[string]error)
			}
			handleErrors[sec] = fmt.Errorf("secret handle '%s' was not resolved by the secret_backend_command. Ensure your script returns the handle in the expected JSON format. Docs: %s", sec, secretsManagementDocsURL)
			continue
		}

		if v.ErrorMsg != "" {
			r.tlmSecretResolveError.Inc("error", sec)
			if handleErrors == nil {
				handleErrors = make(map[string]error)
			}
			handleErrors[sec] = fmt.Errorf("an error occurred while resolving '%s': %s", sec, v.ErrorMsg)
			continue
		}

		if r.removeTrailingLinebreak {
			v.Value = strings.TrimRight(v.Value, "\r\n")
		}

		if v.Value == "" {
			r.tlmSecretResolveError.Inc("empty", sec)
			if handleErrors == nil {
				handleErrors = make(map[string]error)
			}
			handleErrors[sec] = fmt.Errorf("resolved secret for '%s' is empty. Check that the secret exists in your backend and has a non-empty value. If using secret_backend_remove_trailing_line_break, trailing newlines are stripped. Docs: %s", sec, secretsManagementDocsURL)
			continue
		}
		resolved[sec] = v.Value
	}
	return resolved, handleErrors, nil
}
