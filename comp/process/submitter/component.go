// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package submitter implements a component to submit collected data in the Process Agent to
// supported Datadog intakes.
package submitter

import (
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// team: processes

// Component is the component type.
type Component interface {
	processRunner.Submitter
}
