// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package mock

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
)

type mockFileOffsetStorage struct {
	offset int64
}

// GetLastCommittedOffset returns the offset passed at initialization
func (s *mockFileOffsetStorage) GetLastCommittedOffset(identifier string) (int64, int) {
	return s.offset, os.SEEK_CUR
}

// NewFileOffsetStorage returns a new mocked FileOffsetStorage
func NewFileOffsetStorage(offset int) auditor.FileOffsetStorage {
	return &mockFileOffsetStorage{
		offset: int64(offset),
	}
}
