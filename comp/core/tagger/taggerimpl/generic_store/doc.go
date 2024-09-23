// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package genericstore defines a generic object store that satisfies a redundant use-case in the tagger component implementation.
// The implementation of the tagger component requires storing objects indexed by keys.
// Keys are in the form of `{prefix}://{id}`.
//
// The package provides a generic interface ObjectStore which can store objects of a given type and index by tagger EntityID (i.e. a prefix + an id).
// It also provides 2 implementations of this interface:
// - defaultObjectStore: implements the object store as a plain from entity id to entity object. It is intended to be used when EntityID is stored as a string.
// - compositeObjectStore: implements the object store as a 2-layered map. The first map is indexed by prefix, and the second map is indexed by id. It is intended to be used when EntityID is stored
// as a struct separating prefix and id into 2 fields. This implementation is optimised for quick lookups, listing and filtering by prefix.
package genericstore
