// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers defines default secrets providers
package providers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
)

const (
	maxSecretFileSize = 8192
)

// ReadSecretFile reads the given secret file
func ReadSecretFile(path string) secrets.SecretVal {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return secrets.SecretVal{Value: "", ErrorMsg: "secret does not exist"}
		}
		return secrets.SecretVal{Value: "", ErrorMsg: err.Error()}
	}

	// In kubernetes when kubelet mounts the secret|configmap key as a file, it
	// is always a symlink to allow “atomic update“.
	if fi.Mode()&os.ModeSymlink != 0 {
		// Check that the symlink is in the same dir.  This is not a security measure, but just a
		// sanity check.
		target, err := os.Readlink(path)
		if err != nil {
			return secrets.SecretVal{Value: "", ErrorMsg: fmt.Sprintf("failed to read symlink target: %v", err)}
		}

		dir := filepath.Dir(path)
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(filepath.Join(dir, target))
			if err != nil {
				return secrets.SecretVal{Value: "", ErrorMsg: fmt.Sprintf("failed to resolve symlink absolute path: %v", err)}
			}
		}

		targetDir := filepath.Dir(target)

		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			return secrets.SecretVal{Value: "", ErrorMsg: fmt.Sprintf("failed to resolve absolute path of directory: %v", err)}
		}

		if !strings.HasPrefix(targetDir+"/", dirAbs+"/") {
			return secrets.SecretVal{Value: "", ErrorMsg: fmt.Sprintf("not following symlink %q outside of %q", target, dir)}
		}
	}
	fi, err = os.Stat(path)
	if err != nil {
		return secrets.SecretVal{Value: "", ErrorMsg: err.Error()}
	}

	if fi.Size() > maxSecretFileSize {
		return secrets.SecretVal{Value: "", ErrorMsg: "secret exceeds max allowed size"}
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return secrets.SecretVal{Value: "", ErrorMsg: err.Error()}
	}

	return secrets.SecretVal{Value: string(bytes), ErrorMsg: ""}
}

// ReadSecretFileConfined reads a secret within the configured directory.
func ReadSecretFileConfined(rootPath, path string) secrets.SecretVal {
	if rootPath == "" {
		return secrets.SecretVal{ErrorMsg: "secret directory is required"}
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	defer root.Close()

	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}
	if filepath.IsAbs(path) {
		path, err = filepath.Rel(rootAbs, path)
		if err != nil {
			return secrets.SecretVal{ErrorMsg: err.Error()}
		}
	}

	// Root.Open rejects absolute symlinks even when they point inside the root.
	linkInfo, err := root.Lstat(path)
	if err == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(filepath.Join(rootAbs, path))
		if err != nil {
			return secrets.SecretVal{ErrorMsg: err.Error()}
		}
		if filepath.IsAbs(target) {
			path, err = filepath.Rel(rootAbs, target)
			if err != nil {
				return secrets.SecretVal{ErrorMsg: err.Error()}
			}
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
