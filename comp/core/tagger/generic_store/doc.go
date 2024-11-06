// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package genericstore defines a generic object store that satisfies a redundant use-case in the tagger component implementation.
// The implementation of the tagger component requires storing objects indexed by keys.
// Keys are in the form of `{prefix}://{id}`.
package genericstore
