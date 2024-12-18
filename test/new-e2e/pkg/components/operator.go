// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
)

// Operator is a Datadog Operator running in a Kubernetes cluster
type Operator struct {
	operator.OperatorOutput
}
