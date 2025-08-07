// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SymDBUploader interface for uploading SymDB data
type SymDBUploader interface {
	Enqueue(eventMetadata *uploader.EventMetadata, symdbRoot *uploader.SymDBRoot) error
}

type symdbManager struct {
	uploader SymDBUploader
	store    *processStore
}

func newSymdbManager(uploader SymDBUploader, store *processStore) *symdbManager {
	return &symdbManager{
		uploader: uploader,
		store:    store,
	}
}

func (m *symdbManager) requestUpload(runtimeID procRuntimeID, executable actuator.Executable) {
	if !m.store.markSymdbUploadStarted(runtimeID.ProcessID) {
		return
	}

	go m.performUpload(runtimeID, executable)
}

func (m *symdbManager) performUpload(runtimeID procRuntimeID, executable actuator.Executable) {
	builder, err := symdb.NewSymDBBuilder(executable.Path, symdb.ExtractScopeMainModuleOnly)
	if err != nil {
		log.Errorf("SymDB: Failed to create builder for process %v (executable: %s): %v",
			runtimeID.ProcessID, executable.Path, err)
		return
	}
	defer builder.Close()

	symbols, err := builder.ExtractSymbols()
	if err != nil {
		log.Errorf("SymDB: Failed to extract symbols for process %v (executable: %s): %v",
			runtimeID.ProcessID, executable.Path, err)
		return
	}

	eventMetadata := uploader.NewEventMetadata(runtimeID.service, runtimeID.runtimeID)
	symdbRoot := uploader.NewSymDBRoot(runtimeID.service, runtimeID.environment, runtimeID.version, symbols)
	if err := m.uploader.Enqueue(eventMetadata, symdbRoot); err != nil {
		log.Errorf("SymDB: Failed to enqueue symbols for process %v (executable: %s): %v",
			runtimeID.ProcessID, executable.Path, err)
		return
	}

	log.Tracef("SymDB: Successfully enqueued symbols for process %v (executable: %s), main module: %s, packages: %d",
		runtimeID.ProcessID, executable.Path, symbols.MainModule, len(symbols.Packages))
}
