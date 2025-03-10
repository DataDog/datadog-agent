// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "C"

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle"
)

func main() {}

// CheckFactory exports the Oracle check factory
//
//export CheckFactory
func CheckFactory() unsafe.Pointer {
	factory := oracle.Factory
	return unsafe.Pointer(&factory)
}

// go build -tags oracle -o oracle.so -buildmode=c-shared ./pkg/collector/corechecks/oracle/main/main.go
