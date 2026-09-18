// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build tools

//go:generate go run github.com/DataDog/orchestrion pin -generate

package orchestrion

import (
	_ "github.com/DataDog/dd-trace-go/v2/internal/civisibility/integrations/gotesting" // integration
	_ "github.com/DataDog/orchestrion"                                                 // integration

	_ "github.com/DataDog/dd-trace-go/orchestrion/all/v2" // integration
)
