// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"github.com/DataDog/datadog-agent/comp/process/submitter"
)

// Check defines an interface implemented by checks
type Check interface {
	IsEnabled() bool
	Name() string
	Run() (*submitter.Payload, error)
}
