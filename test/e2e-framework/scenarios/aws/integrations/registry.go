// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package integrations aggregates the agint:generate-lab integration labs so the
// top-level scenario registry only needs a single entry for all of them. New labs
// add one scenario package under this directory and one line in Scenarios().
package integrations

import (
	awsneuron "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/aws_neuron"
	dellpowerflex "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/dell_powerflex"
	awsetcd "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/kafka"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/lustre"
	awspostgres "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/postgres"
	awsredisdb "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/integrations/redisdb"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Scenarios returns the registry entries for every integration lab, keyed
// "aws/integrations/<integration>".
func Scenarios() map[string]pulumi.RunFunc {
	return map[string]pulumi.RunFunc{
		"aws/integrations/redisdb":        awsredisdb.VMRun,
		"aws/integrations/postgres":       awspostgres.VMRun,
		"aws/integrations/kafka":          kafka.VMRun,
		"aws/integrations/etcd":           awsetcd.Run,
		"aws/integrations/aws_neuron":     awsneuron.Run,
		"aws/integrations/lustre":         lustre.VMRun,
		"aws/integrations/dell_powerflex": dellpowerflex.VMRun,
	}
}
