// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

import "github.com/DataDog/datadog-agent/pkg/collector/check"

// CheckVisitor defines a visitor func for compliance checks
type CheckVisitor func(check.Check) error
