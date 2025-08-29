// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import "github.com/DataDog/datadog-agent/pkg/dyninst/object"

// ObjectLoader is an interface that abstracts the loading of object files.
type ObjectLoader interface {
	Load(path string) (object.FileWithDwarf, error)
}

type config struct {
	maxDynamicTypeSize uint32
	maxHashBucketsSize uint32
	objectLoader       ObjectLoader
}

var defaultConfig = config{
	maxDynamicTypeSize: defaultMaxDynamicTypeSize,
	maxHashBucketsSize: defaultMaxHashBucketsSize,
	objectLoader:       object.NewInMemoryLoader(),
}

// This is an arbitrary limit for how much data will be captured for
// dynamically sized types (strings and slices).
const defaultMaxDynamicTypeSize = 2048

// Same limit, but for hashmap buckets slice (both hmaps and swiss maps,
// both using pointers and embedded key/value types). Limit is higher
// than for strings and slices, given that not all bucket slots are
// occupied.
const defaultMaxHashBucketsSize = 4 * defaultMaxDynamicTypeSize

// Option configures ir generation.
type Option interface {
	apply(c *config)
}

type maxDynamicDataSizeOption uint32

func (o maxDynamicDataSizeOption) apply(c *config) {
	c.maxDynamicTypeSize = uint32(o)
	c.maxHashBucketsSize = uint32(o) * 4
}

// WithMaxDynamicDataSize sets the maximum size of dynamically sized types
// (strings and slices), it also configures the amount of data that will be
// captured for hashmap buckets to be 4x the size of the dynamically sized
// types.
func WithMaxDynamicDataSize(size int) Option {
	return maxDynamicDataSizeOption(size)
}

// WithObjectLoader sets the object loader to use for loading object files.
func WithObjectLoader(loader ObjectLoader) Option {
	return optionFunc(func(c *config) { c.objectLoader = loader })
}

type optionFunc func(c *config)

func (o optionFunc) apply(c *config) { o(c) }
