// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build secrets

package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const payloadVersion = "1.0"

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

	cmd := exec.CommandContext(ctx, secretBackendCommand, secretBackendArguments...)
	if err := checkRights(cmd.Path); err != nil {
		return nil, err
	}

	cmd.Stdin = strings.NewReader(inputPayload)

	stdout := limitBuffer{
		buf: &bytes.Buffer{},
		max: secretBackendOutputMaxSize,
	}
	stderr := limitBuffer{
		buf: &bytes.Buffer{},
		max: secretBackendOutputMaxSize,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Errorf("secret_backend_command stderr: %s", stderr.buf.String())

		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("error while running '%s': command timeout", secretBackendCommand)
		}
		return nil, fmt.Errorf("error while running '%s': %s", secretBackendCommand, err)
	}
	return stdout.buf.Bytes(), nil
}

type secret struct {
	Value    string
	ErrorMsg string `json:"error"`
}

// for testing purpose
var runCommand = execCommand

// fetchSecret receives a list of secrets name to fetch, exec a custom
// executable to fetch the actual secrets and returns them. Origin should be
// the name of the configuration where the secret was referenced.
func fetchSecret(secretsHandle []string, origin string) (map[string]string, error) {
	payload := map[string]interface{}{
		"version": payloadVersion,
		"secrets": secretsHandle,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("could not serialize secrets IDs to fetch password: %s", err)
	}
	log.Debugf("calling secret_backend_command with payload: '%s'", jsonPayload)
	output, err := runCommand(string(jsonPayload))
	if err != nil {
		return nil, err
	}

	secrets := map[string]secret{}
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
