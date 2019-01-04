// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build android

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/util/androidasset"
)

var (
	readDirPtr  = androidasset.ReadDir
	readFilePtr = androidasset.ReadFile
)
