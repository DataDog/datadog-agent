// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SymDBUploader interface for uploading SymDB data
type SymDBUploader interface {
	Enqueue(pkg symdb.Package) error
}

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

	go m.performUpload(runtimeID, executable)
}

func (m *symdbManager) performUpload(runtimeID procRuntimeID, executable actuator.Executable) {
	it, err := symdb.PackagesIterator(executable.Path,
		symdb.ExtractOptions{
			Scope:                   symdb.ExtractScopeModulesFromSameOrg,
			IncludeInlinedFunctions: false,
		})
	if err != nil {
		log.Errorf("SymDB: failed to read symbols for process %v (executable: %s): %v",
			runtimeID.ProcessID, executable.Path, err)
		return
	}

	up := uploader.NewSymDBUploader(
		runtimeID.service, runtimeID.environment, runtimeID.version, runtimeID.runtimeID,
		uploader.WithURL(m.uploadURL))
	for pkg, err := range it {
		if err != nil {
			log.Errorf("SymDB: Failed to iterate packages for process %v (executable: %s): %v",
				runtimeID.ProcessID, executable.Path, err)
			return
		}
		if err := up.Enqueue(pkg); err != nil {
			log.Errorf("SymDB: Failed to enqueue symbols for process %v (executable: %s): %v",
				runtimeID.ProcessID, executable.Path, err)
			return
		}
	}

	log.Tracef("SymDB: Successfully enqueued symbols for process %v (executable: %s)",
		runtimeID.ProcessID, executable.Path)
}
