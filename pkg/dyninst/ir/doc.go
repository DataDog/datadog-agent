// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ir contains a representation of probes as applied
// to a specific binary.
//
// The key data structure is ir.Program, which represents all the
// information needed to describe the set of probes that will be
// connected to a single binary.
package ir
