// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides tools to manage environments and run E2E tests.
// See [Suite] for an example of the usage.
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/aws"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Suite manages the environment creation and runs E2E tests.
// It is implemented as a [testify Suite].
// Example of usage:
//
//	  type MyEnv struct {
//		   VM *client.VM
//	  }
//	  type vmSuite struct {
//		   *Suite[MyEnv]
//	  }
//
//	  func TestE2ESuite(t *testing.T) {
//		   suite.Run(t, &vmSuite{Suite: NewSuite("my-test", &StackDefinition[MyEnv]{
//		     EnvCloudName: "aws/sandbox",
//			 EnvFactory: func(ctx *pulumi.Context) (*MyEnv, error) {
//				vm, err := ec2vm.NewUnixLikeEc2VM(ctx, ec2vm.WithOS(os.AmazonLinuxOS, commonos.AMD64Arch))
//				if err != nil {
//					return nil, err
//				}
//				return &MyEnv{
//					VM: client.NewVM(vm),
//				}, nil
//			  },
//		   })})
//	  }
//
// Suite leverages pulumi features to compute the differences between the previous
// environment and the new one to make environment updates faster.
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
type Suite[Env any] struct {
	shortStackName string
	fullStackName  string // From StackManager.Stack. Use shortStackName and the username
	stackDef       *StackDefinition[Env]
	destroyEnv     bool

	// These fields are initialized in SetupSuite
	suite.Suite
	Env              *Env
	auth             client.Authentification
	defaultConfigMap auto.ConfigMap
}

type StackDefinition[Env any] struct {
	EnvCloudName string // Must be "aws/sandbox" for now.
	EnvFactory   func(ctx *pulumi.Context) (*Env, error)
	ConfigMap    auto.ConfigMap
}

// NewSuite creates a new Suite.
// stackName is the name of the stack and should be unique across suites.
// stackDef is the stack definition.
// options are optional parameters for example [e2e.KeepEnv].
func NewSuite[Env any](stackName string, stackDef *StackDefinition[Env], options ...func(*Suite[Env])) *Suite[Env] {
	testSuite := Suite[Env]{
		shortStackName:   stackName,
		stackDef:         stackDef,
		destroyEnv:       true,
		defaultConfigMap: make(auto.ConfigMap),
	}

	for _, o := range options {
		o(&testSuite)
	}

	return &testSuite
}

// KeepEnv prevents Suite from destroying the environment at the end of the test suite.
// When using this option, you have to manually destroy the environment.
func KeepEnv[Env any]() func(*Suite[Env]) {
	return func(p *Suite[Env]) { p.destroyEnv = false }
}

// SetupSuite method will run before the tests in the suite are run.
// This function is called by [testify Suite].
// Note: Having initialization code in this function allows `NewSuite` to not
// return an error in order to write a single line for
// `suite.Run(t, &vmSuite{Suite: e2e.NewSuite(...)})`
func (suite *Suite[Env]) SetupSuite() {
	require := require.New(suite.T())

	// Check if the Env type is correct otherwise raises an error before creating the env.
	err := client.CheckEnvStructValid[Env]()
	require.NoError(err)

	credentialsManager := credentials.NewManager()
	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(err)

	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	require.NoError(err)

	suite.auth.SSHKey = sshKey
	suite.add(config.DDAgentConfigNamespace, config.DDAgentAPIKeyParamName, apiKey)
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultKeyPairParamName, "agent-ci-sandbox")
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultInstanceTypeParamName, "t3.large")
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultARMInstanceTypeParamName, "m6g.medium")
	env, stack, upResult, err := createEnv(suite, suite.stackDef)
	require.NoError(err)
	suite.fullStackName = stack.Name()
	suite.Env = env
	err = client.CallStackInitializers(&suite.auth, env, upResult)
	require.NoError(err)
}

func createEnv[Env any](suite *Suite[Env], stackDef *StackDefinition[Env]) (*Env, *auto.Stack, auto.UpResult, error) {
	var env *Env
	ctx := context.Background()
	configMap := auto.ConfigMap{}
	for key, value := range suite.defaultConfigMap {
		configMap[key] = value
	}

	// Override the values of the config map with the values from the StackDefinition.
	for key, value := range stackDef.ConfigMap {
		configMap[key] = value
	}
	stack, stackOutput, err := infra.GetStackManager().GetStack(
		ctx,
		suite.stackDef.EnvCloudName,
		suite.shortStackName,
		configMap,
		func(ctx *pulumi.Context) error {
			var err error
			env, err = stackDef.EnvFactory(ctx)
			return err
		}, false)

	return env, stack, stackOutput, err
}

func (c *Suite[Env]) add(namespace string, key string, value string) {
	c.defaultConfigMap[namespace+":"+key] = auto.ConfigValue{Value: value}
}

// TearDownSuite method is run after all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite[Env]) TearDownSuite() {
	var err error

	if suite.fullStackName == "" {
		// There was an error when creating the stack so nothing to destroy
		return
	}

	if suite.destroyEnv {
		ctx := context.Background()
		err = infra.GetStackManager().DeleteStack(ctx, suite.stackDef.EnvCloudName, suite.shortStackName)
	}

	if !suite.destroyEnv || err != nil {
		stars := strings.Repeat("*", 50)
		thisFolder := "A_FOLDER_CONTAINING_PULUMI.YAML"
		if _, thisFile, _, ok := runtime.Caller(0); ok {
			thisFolder = filepath.Dir(thisFile)
		}

		command := fmt.Sprintf("pulumi destroy -C %v --remove  -s %v", thisFolder, suite.fullStackName)
		fmt.Fprintf(os.Stderr, "\n%v\nYour environment was not destroyed.\nTo destroy it, run `%v`.\n%v", stars, command, stars)
	}
	require.NoError(suite.T(), err)
}
