// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/kr/pretty"
)

type WriterSerializer[T any] struct {
	output io.Writer
	mu     sync.Mutex
}

func NewWriterLogSerializer(writer io.Writer) (*WriterSerializer[ditypes.SnapshotUpload], error) {
	if writer == nil {
		return nil, errors.New("nil writer for creating log serializer")
	}
	return NewWriterSerializer[ditypes.SnapshotUpload](writer)
}

func NewWriterDiagnosticSerializer(dm *diagnostics.DiagnosticManager, writer io.Writer) (*WriterSerializer[ditypes.DiagnosticUpload], error) {
	if writer == nil {
		return nil, errors.New("nil writer for creating diagnostic serializer")
	}
	ds, err := NewWriterSerializer[ditypes.DiagnosticUpload](writer)
	if err != nil {
		return nil, err
	}
	go func() {
		for diagnostic := range dm.Updates {
			err = ds.Enqueue(diagnostic)
			if err != nil {
				log.Errorf("diagnostic update not enqueued %v: %s", err, pretty.Sprint(diagnostic))
			}
		}
	}()
	return ds, nil
}

func NewWriterSerializer[T any](writer io.Writer) (*WriterSerializer[T], error) {
	if writer == nil {
		return nil, errors.New("nil writer for creating serializer")
	}
	return &WriterSerializer[T]{
		output: writer,
	}, nil
}

func (s *WriterSerializer[T]) Enqueue(item *T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bs, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("Failed to marshal item %v", item)
	}

	_, err = s.output.Write(bs)
	if err != nil {
		return err
	}
	return nil
}
