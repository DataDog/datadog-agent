// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package e2e provides tools to manage environments and run E2E tests.
// See [Suite] for an example of the usage.
package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/aws"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/vm"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ssh"
)

// Suite manages the environment creation and runs E2E tests.
// It is implemented as a [testify Suite] and each test defines the environment and the E2E logic.
// Example of usage:
//
//	type vmSuite struct {
//		e2e.Suite
//	}
//
//	func TestMyEnv(t *testing.T) {
//		suite.Run(t, &vmSuite{Suite: e2e.NewSuite("aws/sandbox", "my-test")})
//	}
//
//	func (v *vmSuite) Test1() {
//		v.Run(
//			func(ctx *pulumi.Context) (*vm.VM, error) {
//				return ec2vm.NewEc2VM(ctx) // The environment to create. An Empty VM in this example.
//			},
//			func(client *e2e.Client) { // The E2E test.
//				output := client.Execute("ls /tmp")
//				require.NotEmpty(v.T(), output)
//			},
//		)
//	}
//
// Suite leverages pulumi features to compute the differences between the previous
// environment and the new one to make environment updates faster.
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
type Suite struct {
	envName       string
	stackName     string
	ctx           context.Context
	destroyEnv    bool
	fullStackName string

	// These fields are initialized in SetupTest
	suite.Suite
	configMap auto.ConfigMap
	require   *require.Assertions
	sshKey    string
}

// KeepEnv prevents [Suite] from destroying the environment at the end of the test suite.
// When using this option, you have to manually destroy the environment.
func KeepEnv() func(*Suite) {
	return func(p *Suite) { p.destroyEnv = false }
}

// NewSuite creates a new [Suite].
// envName can be either "aws/sandbox" or "az/sandbox".
// stackName is the name of the stack and should be unique.
// options are optional parameters for example [e2e.KeepEnv].
func NewSuite(envName string, stackName string, options ...func(*Suite)) Suite {
	testSuite := Suite{
		envName:    envName,
		stackName:  stackName,
		ctx:        context.Background(),
		configMap:  auto.ConfigMap{},
		destroyEnv: true,
	}
	for _, o := range options {
		o(&testSuite)
	}
	return testSuite
}

// SetupTest method is run before each test in the suite.
// This function is called by [testify Suite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite) SetupTest() {
	suite.require = require.New(suite.T())
	credentialsManager := credentials.NewManager()
	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	suite.require.NoError(err)

	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	suite.require.NoError(err)

	suite.sshKey = sshKey
	suite.add(config.DDAgentConfigNamespace, config.DDAgentAPIKeyParamName, apiKey)
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultKeyPairParamName, "agent-ci-sandbox")
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultInstanceTypeParamName, "t3.large")
	suite.add(config.DDInfraConfigNamespace, aws.DDInfraDefaultARMInstanceTypeParamName, "m6g.medium")
}

func (c *Suite) add(namespace string, key string, value string) {
	c.configMap[namespace+":"+key] = auto.ConfigValue{Value: value}
}

// TearDownSuite method is run after all the tests in the suite have been run.
// This function is called by [testify Suite].
//
// [testify Suite]: https://pkg.go.dev/github.com/stretchr/testify/suite
func (suite *Suite) TearDownSuite() {
	if suite.fullStackName != "" {
		var err error
		if suite.destroyEnv {
			err = infra.GetStackManager().DeleteStack(suite.ctx, suite.envName, suite.stackName)
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
		suite.require.NoError(err)
	}
}

// Client provides a client to interact with the environment.
type Client struct {
	require   *require.Assertions
	sshClient *ssh.Client
}

// Execute executes a command on the remote environment.
func (c *Client) Execute(command string) string {
	output, err := clients.ExecuteCommand(c.sshClient, command)
	c.require.NoError(err, "Command `%v` failed with the output `%v`", command, output)
	return output
}

// Run creates an environment and runs the E2E test.
// The first argument is a function that creates the environment. It must return the type ([*vm.VM], error).
// The second argument is a function that implements the E2E test logic.
func (suite *Suite) Run(envFactory func(ctx *pulumi.Context) (*vm.VM, error), testFct func(*Client)) {
	sshClient, err := suite.createEnv(envFactory)
	suite.require.NoError(err)
	defer sshClient.Close()
	client := &Client{require: suite.require, sshClient: sshClient}
	testFct(client)
}

func (suite *Suite) createEnv(fct func(ctx *pulumi.Context) (*vm.VM, error)) (*ssh.Client, error) {
	stack, stackOutput, err := infra.GetStackManager().GetStack(suite.ctx, suite.envName, suite.stackName, suite.configMap, func(ctx *pulumi.Context) error {
		_, err := fct(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	if suite.fullStackName == "" {
		suite.fullStackName = stack.Name()
	}

	suite.require.Equal(suite.fullStackName, stack.Name())
	user, ip, err := getConnection(stackOutput)
	if err != nil {
		return nil, fmt.Errorf("error connecting to your environment: %v", err)
	}

	client, _, err := clients.GetSSHClient(user, fmt.Sprintf("%s:%d", ip, 22), suite.sshKey, 2*time.Second, 5)
	return client, err
}

func getConnection(stackOutput auto.UpResult) (string, string, error) {
	connection, found := stackOutput.Outputs["connection"]
	if !found {
		return "", "", errors.New("connection not found")
	}

	values, ok := connection.Value.(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("invalid type for connection: %v", reflect.TypeOf(connection.Value))
	}

	ip, err := getMapStringValue(values, "host")
	if err != nil {
		return "", "", err
	}

	user, err := getMapStringValue(values, "user")
	return user, ip, err
}

func getMapStringValue(values map[string]interface{}, key string) (string, error) {
	value, found := values[key]
	if !found {
		return "", fmt.Errorf("%v not found", key)
	}
	result, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("invalid type for %v: %v. It must be `string`", key, reflect.TypeOf(key))
	}
	return result, nil
}
