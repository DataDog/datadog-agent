// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testdata

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func A() {
	pkgconfigmodel.NewConfig("test", "DD", nil) // want "error detected on method call"
	fmt.Printf("test %s", "test")               // want "Printf detected"
}

func B() {
	model.NewConfig("test", "DD", nil) // want "error detected on method call"
}
