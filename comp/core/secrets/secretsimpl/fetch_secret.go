// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

func (r *secretResolver) execCommand(inputPayload string) ([]byte, error) {
	// hook used only for tests
	if r.commandHookFunc != nil {
		return r.commandHookFunc(inputPayload)
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(r.backendTimeout)*time.Second)
	defer cancel()

	cmd, done, err := commandContext(ctx, r.backendCommand, r.backendArguments...)
	if err != nil {
		return nil, err
	}
	defer done()

	if err := checkRights(cmd.Path, r.commandAllowGroupExec); err != nil {
		return nil, err
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
			return nil, fmt.Errorf("error while running '%s': command timeout", r.backendCommand)
		}
		return nil, fmt.Errorf("error while running '%s': %s", r.backendCommand, err)
	}

	log.Debugf("secret_backend_command stderr: %s", stderr.buf.String())

	r.tlmSecretBackendElapsed.Add(float64(elapsed.Milliseconds()), r.backendCommand, "0")
	return stdout.buf.Bytes(), nil
}

// fetchSecret receives a list of secrets name to fetch, exec a custom
// executable to fetch the actual secrets and returns them.
func (r *secretResolver) fetchSecret(secretsHandle []string) (map[string]string, error) {
	payload := map[string]interface{}{
		"version": secrets.PayloadVersion,
		"secrets": secretsHandle,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not serialize secrets IDs to fetch password: %s", err)
	}
	output, err := r.execCommand(string(jsonPayload))
	if err != nil {
		return nil, err
	}

	secrets := map[string]secrets.SecretVal{}
	err = json.Unmarshal(output, &secrets)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal 'secret_backend_command' output: %s", err)
	}

	res := map[string]string{}
	for _, sec := range secretsHandle {
		v, ok := secrets[sec]
		if !ok {
			return nil, fmt.Errorf("secret handle '%s' was not resolved by the secret_backend_command", sec)
		}

		if v.ErrorMsg != "" {
			return nil, fmt.Errorf("an error occurred while resolving '%s': %s", sec, v.ErrorMsg)
		}

		if r.removeTrailingLinebreak {
			v.Value = strings.TrimRight(v.Value, "\r\n")
		}

		if v.Value == "" {
			return nil, fmt.Errorf("resolved secret for '%s' is empty", sec)
		}
		res[sec] = v.Value
	}
	return res, nil
}
