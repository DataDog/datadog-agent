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

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// OfflineSerializer is used for serializing events and printing instead of
// uploading to the DataDog backend
type OfflineSerializer[T any] struct {
	outputFile *os.File
	mu         sync.Mutex
}

// NewOfflineLogSerializer creates an offline serializer for serializing events and printing instead of
// uploading to the DataDog backend
func NewOfflineLogSerializer(outputPath string) (*OfflineSerializer[ditypes.SnapshotUpload], error) {
	if outputPath == "" {
		panic("No snapshot output path set")
	}
	return NewOfflineSerializer[ditypes.SnapshotUpload](outputPath)
}

// NewOfflineDiagnosticSerializer creates an offline serializer for serializing diagnostic information
// and printing instead of uploading to the DataDog backend
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

// NewOfflineSerializer is the generic create method for offline serialization
// of events or diagnostic output
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

// Enqueue writes data to the offline serializer
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
