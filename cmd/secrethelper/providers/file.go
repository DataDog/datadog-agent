// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package providers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	s "github.com/DataDog/datadog-agent/pkg/secrets"
)

const (
	maxSecretFileSize = 8192
)

// ReadSecretFile TODO <agent-core>
func ReadSecretFile(path string) s.Secret {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.Secret{Value: "", ErrorMsg: "secret does not exist"}
		}
		return s.Secret{Value: "", ErrorMsg: err.Error()}
	}

	// In kubernetes when kubelet mounts the secret|configmap key as a file, it
	// is always a symlink to allow “atomic update“.
	if fi.Mode()&os.ModeSymlink != 0 {
		// Check that the symlink is in the same dir.  This is not a security measure, but just a
		// sanity check.
		target, err := os.Readlink(path)
		if err != nil {
			return s.Secret{Value: "", ErrorMsg: fmt.Sprintf("failed to read symlink target: %v", err)}
		}

		dir := filepath.Dir(path)
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(filepath.Join(dir, target))
			if err != nil {
				return s.Secret{Value: "", ErrorMsg: fmt.Sprintf("failed to resolve symlink absolute path: %v", err)}
			}
		}

		targetDir := filepath.Dir(target)

		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			return s.Secret{Value: "", ErrorMsg: fmt.Sprintf("failed to resolve absolute path of directory: %v", err)}
		}

		if !strings.HasPrefix(targetDir+"/", dirAbs+"/") {
			return s.Secret{Value: "", ErrorMsg: fmt.Sprintf("not following symlink %q outside of %q", target, dir)}
		}
	}
	fi, err = os.Stat(path)
	if err != nil {
		return s.Secret{Value: "", ErrorMsg: err.Error()}
	}

	if fi.Size() > maxSecretFileSize {
		return s.Secret{Value: "", ErrorMsg: "secret exceeds max allowed size"}
	}

	file, err := os.Open(path)
	if err != nil {
		return s.Secret{Value: "", ErrorMsg: err.Error()}
	}

	bytes, err := io.ReadAll(file)
	if err != nil {
		return s.Secret{Value: "", ErrorMsg: err.Error()}
	}

	return s.Secret{Value: string(bytes), ErrorMsg: ""}
}
