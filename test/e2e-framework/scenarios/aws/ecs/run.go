package ecs

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/aspnetsample"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/cpustress"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/dogstatsd"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/nginx"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/prometheus"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/tracegen"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"

	resourcesAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Create cluster
	ecsOpts := buildClusterOptionsFromConfigMap(awsEnv)
	cluster, err := NewCluster(awsEnv, "ecs", ecsOpts...)
	if err != nil {
		return err
	}

	err = cluster.Export(ctx, nil)
	if err != nil {
		return err
	}

	var apiKeyParam *ssm.Parameter
	var fakeIntake *fakeintakeComp.Fakeintake
	// Create task and service
	if awsEnv.AgentDeploy() {
		if awsEnv.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{}
			if awsEnv.InfraShouldDeployFakeintakeWithLB() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
			}

			if storeType := awsEnv.AgentFakeintakeStoreType(); storeType != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithStoreType(storeType))
			}

			if retentionPeriod := awsEnv.AgentFakeintakeRetentionPeriod(); retentionPeriod != "" {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithRetentionPeriod(retentionPeriod))
			}

			if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
				return err
			}
			if err := fakeIntake.Export(awsEnv.Ctx(), nil); err != nil {
				return err
			}
		}
		apiKeyParam, err = ssm.NewParameter(ctx, awsEnv.Namer.ResourceName("agent-apikey"), &ssm.ParameterArgs{
			Name:      awsEnv.CommonNamer().DisplayName(1011, pulumi.String("agent-apikey")),
			Type:      ssm.ParameterTypeSecureString,
			Overwrite: pulumi.Bool(true),
			Value:     awsEnv.AgentAPIKey(),
		}, awsEnv.WithProviders(config.ProviderAWS))
		if err != nil {
			return err
		}

		_, err := agent.ECSLinuxDaemonDefinition(awsEnv, "ec2-linux-dd-agent", apiKeyParam.Name, fakeIntake, cluster.ClusterArn)
		if err != nil {
			return err
		}

	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
		if _, err := nginx.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := redis.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := cpustress.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := dogstatsd.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := prometheus.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}

		if _, err := tracegen.EcsAppDefinition(awsEnv, cluster.ClusterArn); err != nil {
			return err
		}
	}

	// Deploy Fargate Agents
	if awsEnv.TestingWorkloadDeploy() && awsEnv.AgentDeploy() {
		if _, err := redis.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}

		if _, err = nginx.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}

		if _, err = aspnetsample.FargateAppDefinition(awsEnv, cluster.ClusterArn, apiKeyParam.Name, fakeIntake); err != nil {
			return err
		}
	}

	return nil
}
