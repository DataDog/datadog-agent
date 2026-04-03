// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
)

// persistentCacheSaver implements evtbookmark.Saver using the Agent's persistent cache.
type persistentCacheSaver struct {
	key string
}

// newPersistentCacheSaver creates a new persistentCacheSaver with the given cache key.
func newPersistentCacheSaver(key string) evtbookmark.Saver {
	return &persistentCacheSaver{key: key}
}

// Save writes the bookmark XML to the persistent cache.
func (s *persistentCacheSaver) Save(bookmarkXML string) error {
	err := persistentcache.Write(s.key, bookmarkXML)
	if err != nil {
		return fmt.Errorf("failed to write bookmark to persistent cache: %w", err)
	}
	return nil
}

// Load reads the bookmark XML from the persistent cache.
func (s *persistentCacheSaver) Load() (string, error) {
	bookmarkXML, err := persistentcache.Read(s.key)
	if err != nil {
		// persistentcache.Read() does not return error if key does not exist,
		// but we'll handle any other errors
		return "", fmt.Errorf("failed to read bookmark from persistent cache: %w", err)
	}
	return bookmarkXML, nil
}
