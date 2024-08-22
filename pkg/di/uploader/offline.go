// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package uploader

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
)

type OfflineSerializer[T any] struct {
	outputFile *os.File
	mu         sync.Mutex
}

func NewOfflineLogSerializer(outputPath string) (*OfflineSerializer[ditypes.SnapshotUpload], error) {
	if outputPath == "" {
		panic("No snapshot output path set")
	}
	return NewOfflineSerializer[ditypes.SnapshotUpload](outputPath)
}

func NewOfflineDiagnosticSerializer(dm *diagnostics.DiagnosticManager, outputPath string) (*OfflineSerializer[ditypes.DiagnosticUpload], error) {
	if outputPath == "" {
		panic("No diagnostic output path set")
	}
	ds, err := NewOfflineSerializer[ditypes.DiagnosticUpload](outputPath)
	if err != nil {
		return nil, err
	}
	go func() {
		for diagnostic := range dm.Updates {
			ds.Enqueue(diagnostic)
		}
	}()
	return ds, nil
}

func NewOfflineSerializer[T any](outputPath string) (*OfflineSerializer[T], error) {
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	u := &OfflineSerializer[T]{
		outputFile: file,
	}
	return u, nil
}

func (s *OfflineSerializer[T]) Enqueue(item *T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	bs, err := json.Marshal(item)
	if err != nil {
		log.Info("Failed to marshal item", item)
		return false
	}

	_, err = s.outputFile.WriteString(string(bs) + "\n")
	if err != nil {
		log.Error(err)
	}
	return true
}
