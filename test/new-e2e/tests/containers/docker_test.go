package containers

import (
	"context"
	"fmt"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	dockervm "github.com/DataDog/test-infra-definitions/scenarios/aws/dockerVM"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"testing"
)

type DockerSuite struct {
	baseSuite
	fullAgentImagePath string
}

func TestDockerSuite(t *testing.T) {
	// Full Agent
	suite.Run(t, &DockerSuite{fullAgentImagePath: "gcr.io/datadoghq/agent:latest"})

	// DogstatsD Standalone
	suite.Run(t, &DockerSuite{fullAgentImagePath: "datadog/dogstatsd:latest"})
}

func (suite *DockerSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddagent:fullImagePath": auto.ConfigValue{Value: suite.fullAgentImagePath},
	}

	stackName := "docker-stack"

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, stackName, stackConfig, dockervm.Run, false)

	if !suite.Assert().NoError(err) {
		_, err := infra.GetStackManager().GetPulumiStackName(stackName)
		suite.Require().NoError(err)
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, stackName)
		}
		suite.T().FailNow()
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))

	suite.baseSuite.SetupSuite()
}

func (suite *DockerSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()
}

func (suite *DockerSuite) TestDSDWithUDS() {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric.uds",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_name:metric-sender-uds`,
				`^short_image`,
				`^image_name`,
				`^image_id`,
				`^docker_image`,
				`^container_id`,
			},
		},
	})
}
