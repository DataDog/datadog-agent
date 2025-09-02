// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gotype contains logic to parse out the type information from a Go
// object file.
//
// Beware, this package reverse engineers the type information from a Go object
// file and relies on internal details of the Go compiler.
package gotype
