// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package reorderer

import (
	"sync"

	"github.com/cilium/ebpf/perf"
)

// RecordPool defines a perf record pool
type RecordPool struct {
	pool sync.Pool
}

// Get returns a record
func (p *RecordPool) Get() *perf.Record {
	return p.pool.Get().(*perf.Record)
}

// Release a record
func (p *RecordPool) Release(record *perf.Record) {
	p.pool.Put(record)
}

// NewRecordPool returns a new RecordPool
func NewRecordPool() *RecordPool {
	return &RecordPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &perf.Record{}
			},
		},
	}
}
