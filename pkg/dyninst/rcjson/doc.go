// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rcjson contains the data structures for remote configuration for the
// DI service.
//
// Note that the structures defined here were derived from the internal dd-go
// representation [1]. Some any fields were replaced with json.RawMessage to
// avoid unnecessary allocations.
//
// [1]: https://github.com/DataDog/dd-go/blob/421dbec4/remote-config/pkg/products/livedebugging/domain.go
package rcjson

// TODO: Automatically generate these structures from the dd-go representation
// to keep them in sync.
