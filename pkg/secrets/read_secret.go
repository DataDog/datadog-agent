// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build secrets

package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	maxSecretFileSize    = 1024
	compatibleMajVersion = "1"
)

// ReadSecretsCmd implements a secrets backend command reading secrets from a directory/mount
var ReadSecretCmd = &cobra.Command{
	Use:   "read-secret",
	Short: "Read secret from a directory",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return ReadSecrets(os.Stdin, os.Stdout, args[0])
	},
}

type secretsRequest struct {
	Version string   `json:"version"`
	Secrets []string `json:"secrets"`
}

// ReadSecrets implements a secrets reader from a directory/mount
func ReadSecrets(r io.Reader, w io.Writer, dir string) error {
	in, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	var request secretsRequest
	err = json.Unmarshal(in, &request)
	if err != nil {
		return errors.New("failed to unmarshal json input")
	}

	version := strings.SplitN(request.Version, ".", 2)
	if version[0] != compatibleMajVersion {
		return fmt.Errorf("incompatible protocol version %q", request.Version)
	}

	if len(request.Secrets) == 0 {
		return errors.New("no secrets listed in input")
	}

	response := map[string]secret{}
	for _, name := range request.Secrets {
		response[name] = readSecret(filepath.Join(dir, name))
	}

	out, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func readSecret(path string) secret {
	value, err := readSecretFile(path)
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	return secret{Value: value, ErrorMsg: errMsg}
}

func readSecretFile(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("secret %q does not exist", path)
		}
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("not following symlink %q", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}

	bufSize := fi.Size()
	if bufSize > maxSecretFileSize {
		return "", fmt.Errorf("secret %q exceeds max file size of %d", path, maxSecretFileSize)
	}
	bytes := make([]byte, bufSize)
	_, err = file.Read(bytes)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
