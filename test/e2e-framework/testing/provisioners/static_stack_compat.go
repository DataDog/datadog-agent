// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package provisioners

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// StaticStackProvisioner moved to the Pulumi-free provisioner package. The
// aliases below keep existing imports working.
type StaticStackProvisioner[Env any] = provisioner.StaticStackProvisioner[Env]

// NewStaticStackProvisioner returns a new StaticStackProvisioner.
func NewStaticStackProvisioner[Env any](id string, filePath string) *StaticStackProvisioner[Env] {
	return provisioner.NewStaticStackProvisioner[Env](id, filePath)
}
