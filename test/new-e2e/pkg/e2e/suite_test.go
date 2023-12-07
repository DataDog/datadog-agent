// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components"
	"github.com/stretchr/testify/require"
)

type testTypeOutput struct {
	components.JSONImporter

	MyField string `json:"myField"`
}

type testTypeWrapper struct {
	testTypeOutput

	unrelatedField string
}

var _ Initializable = &testTypeWrapper{}

func (t *testTypeWrapper) Init(ctx Context) error {
	return nil
}

func (t *testTypeWrapper) GetMyField() string {
	return t.MyField
}

type testEnv struct {
	Wrapper *testTypeWrapper `import:"myWrapper"`
}

type testSuite struct {
	BaseSuite[testEnv]
}

func TestBaseSuite(t *testing.T) {
	suite := &testSuite{}

	env, envFields, envValues, err := suite.createEnv()
	require.NoError(t, err)

	err = suite.buildEnvFromResources(RawResources{
		"myWrapper": []byte(`{"myField":"myValue"}`),
	}, envFields, envValues, env)

	require.NoError(t, err)
	require.Equal(t, "myValue", env.Wrapper.GetMyField())
}
