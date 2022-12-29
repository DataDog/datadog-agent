// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets
// +build secrets

package secrets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// PayloadVersion defines the current payload version sent to a secret backend
	PayloadVersion = "1.0"

	// maxHashFileLimit is the limit for the size of hashing of the binary
	maxHashFileLimit = 1024 * 1024 * 1024 // 1Gi
)

var (
	tlmSecretBackendElapsed = telemetry.NewGauge("secret_backend", "elapsed_ms", []string{"command", "exit_code"}, "Elapsed time of secret backend invocation")
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

func execCommand(inputPayload string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(secretBackendTimeout)*time.Second)
	defer cancel()

	cmd, done, err := commandContext(ctx, secretBackendCommand, secretBackendArguments...)
	if err != nil {
		return nil, err
	}
	defer done()

	if secretBackendCommandSHA256 != "" {
		if !filepath.IsAbs(secretBackendCommand) {
			return nil, fmt.Errorf("error while running '%s': absolute path required with SHA256", secretBackendCommand)
		}

		if err := checkConfigFilePermissions(configFileUsed); err != nil {
			return nil, err
		}

		f, err := lockOpenFile(secretBackendCommand)
		if err != nil {
			return nil, err
		}
		defer f.Close() //nolint:errcheck

		sha256, err := fileHashSHA256(f)
		if err != nil {
			return nil, err
		}

		if !strings.EqualFold(sha256, secretBackendCommandSHA256) {
			return nil, fmt.Errorf("error while running '%s': SHA256 mismatch, actual '%s' expected '%s'", secretBackendCommand, sha256, secretBackendCommandSHA256)
		}

	}

	if err = checkRights(cmd.Path, secretBackendCommandAllowGroupExec); err != nil {
		return nil, err
	}

	cmd.Stdin = strings.NewReader(inputPayload)

	stdout := limitBuffer{
		buf: &bytes.Buffer{},
		max: SecretBackendOutputMaxSize,
	}
	stderr := limitBuffer{
		buf: &bytes.Buffer{},
		max: SecretBackendOutputMaxSize,
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
	log.Debugf("%s | secret_backend_command '%s' completed in %s", time.Now().String(), secretBackendCommand, elapsed)

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
		tlmSecretBackendElapsed.Add(float64(elapsed.Milliseconds()), secretBackendCommand, exitCode)

		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("error while running '%s': command timeout", secretBackendCommand)
		}
		return nil, fmt.Errorf("error while running '%s': %s", secretBackendCommand, err)
	}

	log.Debugf("secret_backend_command stderr: %s", stderr.buf.String())

	tlmSecretBackendElapsed.Add(float64(elapsed.Milliseconds()), secretBackendCommand, "0")
	return stdout.buf.Bytes(), nil
}

// Secret defines the structure for secrets in JSON output
type Secret struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// for testing purpose
var runCommand = execCommand

// fetchSecret receives a list of secrets name to fetch, exec a custom
// executable to fetch the actual secrets and returns them. Origin should be
// the name of the configuration where the secret was referenced.
func fetchSecret(secretsHandle []string, origin string) (map[string]string, error) {
	payload := map[string]interface{}{
		"version": PayloadVersion,
		"secrets": secretsHandle,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not serialize secrets IDs to fetch password: %s", err)
	}
	output, err := runCommand(string(jsonPayload))
	if err != nil {
		return nil, err
	}

	secrets := map[string]Secret{}
	err = json.Unmarshal(output, &secrets)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal 'secret_backend_command' output: %s", err)
	}

	res := map[string]string{}
	for _, sec := range secretsHandle {
		v, ok := secrets[sec]
		if ok == false {
			return nil, fmt.Errorf("secret handle '%s' was not decrypted by the secret_backend_command", sec)
		}

		if v.ErrorMsg != "" {
			return nil, fmt.Errorf("an error occurred while decrypting '%s': %s", sec, v.ErrorMsg)
		}
		if v.Value == "" {
			return nil, fmt.Errorf("decrypted secret for '%s' is empty", sec)
		}

		// add it to the cache
		secretCache[sec] = v.Value
		// keep track of place where a handle was found
		secretOrigin[sec] = common.NewStringSet(origin)
		res[sec] = v.Value
	}
	return res, nil
}

func fileHashSHA256(f *os.File) (string, error) {
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	if !stat.Mode().IsRegular() ||
		(stat.Mode()&(os.ModeSymlink|os.ModeSocket|os.ModeCharDevice|os.ModeDevice|os.ModeNamedPipe) != 0) {
		return "", fmt.Errorf("expecting regular file, got 0x%x", stat.Mode())
	}

	h := sha256.New()

	fileSize := stat.Size()
	if fileSize == 0 {
		return formatHash(h), nil
	}

	if fileSize > maxHashFileLimit {
		return "", fmt.Errorf("file size exceeds the limit of %s", humanize.Bytes(maxHashFileLimit))
	}

	r := io.LimitReader(f, maxHashFileLimit)

	var bufSize int64 = 102400
	if fileSize < bufSize {
		bufSize = fileSize
	}

	buffer := make([]byte, bufSize)

	for {
		read, err := r.Read(buffer)
		if err != nil {
			if err != io.EOF {
				return "", err
			}
			break
		}
		h.Write(buffer[:read])
	}

	return formatHash(h), nil
}

func formatHash(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))
}
