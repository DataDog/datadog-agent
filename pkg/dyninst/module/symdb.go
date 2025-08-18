// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type symdbManager struct {
	uploadURL *url.URL
	store     *processStore
}

func newSymdbManager(uploadURL *url.URL, store *processStore) *symdbManager {
	return &symdbManager{
		uploadURL: uploadURL,
		store:     store,
	}
}

func (m *symdbManager) requestUpload(runtimeID procRuntimeID, executable actuator.Executable) {
	if !m.store.markSymdbUploadStarted(runtimeID.ProcessID) {
		return
	}

	go func() {
		err := m.performUpload(runtimeID, executable)
		if err != nil {
			log.Errorf("SymDB: failed to upload symbols for process %v (executable: %s): %v",
				runtimeID.ProcessID, executable.Path, err)
		}
	}()
}

func (m *symdbManager) performUpload(runtimeID procRuntimeID, executable actuator.Executable) error {
	it, err := symdb.PackagesIterator(executable.Path,
		symdb.ExtractOptions{
			Scope:                   symdb.ExtractScopeModulesFromSameOrg,
			IncludeInlinedFunctions: false,
		})
	if err != nil {
		return fmt.Errorf("failed to read symbols for process %v (executable: %s): %w",
			runtimeID.ProcessID, executable.Path, err)
	}

	sender := uploader.NewSymDBUploader(
		m.uploadURL.String(),
		runtimeID.service, runtimeID.environment, runtimeID.version, runtimeID.runtimeID,
	)
	uploadBuffer := make([]uploader.Scope, 100)
	bufferFuncs := 0
	// Flush every 10k functions to not store too many scopes in memory.
	const maxBufferFuncs = 10000
	maybeFlush := func(force bool) error {
		if len(uploadBuffer) == 0 {
			return nil
		}
		if force || bufferFuncs >= maxBufferFuncs {
			if err := sender.Upload(uploadBuffer); err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}
			uploadBuffer = uploadBuffer[:0]
			bufferFuncs = 0
		}
		return nil
	}
	for pkg, err := range it {
		if err != nil {
			return fmt.Errorf("failed to iterate packages for process %v (executable: %s): %w",
				runtimeID.ProcessID, executable.Path, err)
		}

		scope := uploader.ConvertPackageToScope(pkg)
		uploadBuffer = append(uploadBuffer, scope)
		bufferFuncs += pkg.Stats().NumFunctions
		if err := maybeFlush(false /* force */); err != nil {
			return err
		}
	}
	if err := maybeFlush(true /* force */); err != nil {
		return err
	}

	log.Tracef("SymDB: Successfully uploaded symbols for process %v (executable: %s)",
		runtimeID.ProcessID, executable.Path)
	return nil
}
