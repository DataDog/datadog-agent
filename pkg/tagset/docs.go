// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagset supports creation and manipulation of sets of tags. It
// does so in a safe and efficient fashion, supporting:
//
//     - consistent hashing of tagsets to recognize commonalities
//     - flexible combination of tagsets from multiple sources
//     - immutability to allow re-use of tagsets
//
// The package otherwise presents a fairly abstract API that allows performance
// optimizations without changing semantics.
//
// For background, see the proposal for this package at
// https://github.com/DataDog/datadog-agent/blob/main/docs/proposals/agent-pkg-tagset-rfc.md
//
// Tags
//
// The Tags type is an opaque, immutable data structure representing a set of
// tags. Agent code that handles tags, but does not manipulate them, need only
// use this type.
//
// Factories
//
// Factories are responsible for making new Tags instances. Beneath a simple
// interface, factories support optimization and deduplication. A global factory
// is available for general use, and purpose-specific factories can be created
// for more intensive tag operations.
//
// In general, factories are not thread-safe
//
// Builders
//
// Builders are used to build tagsets tag-by-tag, before "freezing" into one or
// more Tags instances.
package tagset
