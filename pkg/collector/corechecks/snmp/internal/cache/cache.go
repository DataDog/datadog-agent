// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package cache contains the Cacher interface to write snmp tags in cache
package cache

import "github.com/DataDog/datadog-agent/pkg/persistentcache"

// Cacher is the interface to write snmp tags in cache
type Cacher interface {
	Read(key string) (string, error)
	Write(key string, value string) error
}

// PersistentCacher is the implementation of Cacher using persistent cache
type PersistentCacher struct {
}

func (p *PersistentCacher) Read(key string) (string, error) {
	return persistentcache.Read(key)
}

func (p *PersistentCacher) Write(key string, value string) error {
	return persistentcache.Write(key, value)
}
