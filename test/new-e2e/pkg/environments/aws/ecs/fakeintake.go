// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awsdocker contains the definition of the AWS Docker environment.
package awsecs

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"

	"github.com/DataDog/test-infra-definitions/resources/aws"
	awsfakeintake "github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type FakeintakeParams struct {
	// Name is the name of the EC2 VM.
	Name string
	// PulumiContext is the Pulumi context to use.
	PulumiContext *pulumi.Context
	// AwsEnvironment is the AWS environment to use.
	AwsEnvironment *aws.Environment
	// 	FakeintakeOptions is a list of options to configure the FakeIntake.
	FakeintakeOptions []awsfakeintake.Option
	// Importer is the component that will be used to import the fakeintake from pulumi.
	Importer *fakeintake.FakeintakeOutput
}

func NewFakeintake(params FakeintakeParams) (fakeintake *fakeintake.Fakeintake, err error) {
	fakeIntake, err := awsfakeintake.NewECSFargateInstance(*params.AwsEnvironment, params.Name, params.FakeintakeOptions...)
	if err != nil {
		return nil, err
	}
	if params.Importer == nil {
		// nothing to export to
		return fakeIntake, nil
	}
	err = fakeIntake.Export(params.PulumiContext, params.Importer)
	if err != nil {
		return nil, err
	}
	return fakeIntake, nil
}
