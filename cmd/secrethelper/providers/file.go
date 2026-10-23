// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers defines default secrets providers
package providers

import (
	"io"
	"os"
	"path/filepath"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
)

const (
	maxSecretFileSize = 8192
)

// ReadSecretFile reads a secret within the configured directory.
func ReadSecretFile(rootPath, path string) secrets.SecretVal {
	if rootPath == "" {
		return secrets.SecretVal{ErrorMsg: "secret directory is required"}
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	defer root.Close()

	if filepath.IsAbs(path) {
		rootAbs, err := filepath.Abs(rootPath)
		if err != nil {
			return secrets.SecretVal{ErrorMsg: err.Error()}
		}
		path, err = filepath.Rel(rootAbs, path)
		if err != nil {
			return secrets.SecretVal{ErrorMsg: err.Error()}
		}
	}

	file, err := root.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return secrets.SecretVal{ErrorMsg: "secret does not exist"}
		}
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	if fi.Size() > maxSecretFileSize {
		return secrets.SecretVal{ErrorMsg: "secret exceeds max allowed size"}
	}

	bytes, err := io.ReadAll(io.LimitReader(file, maxSecretFileSize+1))
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	if len(bytes) > maxSecretFileSize {
		return secrets.SecretVal{ErrorMsg: "secret exceeds max allowed size"}
	}

	return secrets.SecretVal{Value: string(bytes)}
}
