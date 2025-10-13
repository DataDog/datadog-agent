package gcp

import (
	"fmt"
	"os"
	"os/exec"

	config "github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	gcpConfigNamespace = "gcp"
	gcpNamerNamespace  = "gcp"

	// GCP Infra
	DDInfraDefaultPublicKeyPath            = "gcp/defaultPublicKeyPath"
	DDInfraDefaultPrivateKeyPath           = "gcp/defaultPrivateKeyPath"
	DDInfraDefaultPrivateKeyPassword       = "gcp/defaultPrivateKeyPassword"
	DDInfraDefaultInstanceTypeParamName    = "gcp/defaultInstanceType"
	DDInfraDefaultNetworkNameParamName     = "gcp/defaultNetworkName"
	DDInfraDefaultSubnetNameParamName      = "gcp/defaultSubnet"
	DDInfraDefaultRegionNameParamName      = "gcp/defaultRegion"
	DDInfraDefaultZoneNameParamName        = "gcp/defaultZone"
	DDInfraDefautVMServiceAccountParamName = "gcp/defaultVMServiceAccount"
	DDInfraGKEEnableAutopilot              = "gcp/gke/enableAutopilot"
	DDInfraOpenShiftPullSecretPath         = "gcp/openshift/pullSecretPath"
	DDInfraEnableNestedVirtualization      = "gcp/enableNestedVirtualization"
)

type Environment struct {
	*config.CommonEnvironment

	Namer namer.Namer

	envDefault environmentDefault
}

var _ config.Env = (*Environment)(nil)
var pulumiEnvVariables = []string{"GOOGLE_APPLICATION_CREDENTIALS"}

func NewEnvironment(ctx *pulumi.Context) (Environment, error) {
	env := Environment{
		Namer: namer.NewNamer(ctx, gcpNamerNamespace),
	}
	commonEnv, err := config.NewCommonEnvironment(ctx)
	if err != nil {
		return Environment{}, err
	}
	env.CommonEnvironment = &commonEnv
	env.envDefault = getEnvironmentDefault(config.FindEnvironmentName(commonEnv.InfraEnvironmentNames(), gcpNamerNamespace))

	// TODO: Remove this when we find a better way to automatically log in
	logIn(ctx)

	gcpProvider, err := gcp.NewProvider(ctx, string(config.ProviderGCP), &gcp.ProviderArgs{
		Project: pulumi.StringPtr(env.envDefault.gcp.project),
		Region:  pulumi.StringPtr(env.envDefault.gcp.region),
		Zone:    pulumi.StringPtr(env.envDefault.gcp.zone),
	})
	if err != nil {
		return Environment{}, err
	}
	env.RegisterProvider(config.ProviderGCP, gcpProvider)

	return env, nil
}

func logIn(ctx *pulumi.Context) {
	// Don't log in if the env variables are already set
	envVariablesSet := false
	for _, envVar := range pulumiEnvVariables {
		if os.Getenv(envVar) != "" {
			fmt.Printf("The env variable %s is set\n", envVar)
			envVariablesSet = true
			break
		}
	}

	// Passing the environment variable is not enough for the gcloud CLI to work, per https://cloud.google.com/docs/authentication/provide-credentials-adc
	// We need the CLI to be working for accessing GKE cluster with Pulumi Kubernetes provider.
	if path, ok := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS"); ok {
		cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file", path)
		_, err := cmd.Output()
		if err != nil {
			ctx.Log.Error(fmt.Sprintf("Error running `gcloud auth activate-service-account --key-file %s`: %v", path, err), nil)
		}
	}

	if envVariablesSet {
		return
	}

	cmd := exec.Command("gcloud", "auth", "application-default", "print-access-token")

	// There's no error if the token exists and is still valid
	if err := cmd.Run(); err != nil {
		// Login if the token is not valid anymore
		cmd = exec.Command("gcloud", "auth", "application-default", "login")
		_, err := cmd.Output()

		if err != nil {
			ctx.Log.Error(fmt.Sprintf("Error running `gcloud auth application-default login`: %v", err), nil)
		}
	}
}

// Cross Cloud Provider config

func (e *Environment) InternalRegistry() string {
	return "us-central1-docker.pkg.dev/datadog-agent-qa/agent-qa"
}

func (e *Environment) InternalDockerhubMirror() string {
	return "registry-1.docker.io"
}

func (e *Environment) InternalRegistryImageTagExists(_, _ string) (bool, error) {
	return true, nil
}

func (e *Environment) InternalRegistryFullImagePathExists(_ string) (bool, error) {
	return true, nil
}

// Common

func (e *Environment) DefaultPublicKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPublicKeyPath)
}

func (e *Environment) DefaultPrivateKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPath)
}

func (e *Environment) DefaultPrivateKeyPassword() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPassword)
}

func (e *Environment) DefaultNetworkName() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultNetworkNameParamName, e.envDefault.ddInfra.defaultNetworkName)
}

func (e *Environment) DefaultSubnet() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultSubnetNameParamName, e.envDefault.ddInfra.defaultSubnetName)
}

func (e *Environment) GetCommonEnvironment() *config.CommonEnvironment {
	return e.CommonEnvironment
}
func (e *Environment) DefaultInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceTypeParamName, e.envDefault.ddInfra.defaultInstanceType)
}

func (e *Environment) DefaultVMServiceAccount() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefautVMServiceAccountParamName, e.envDefault.ddInfra.defaultVMServiceAccount)
}

// GKEAutopilot Whether to enable GKE Autopilot or not
func (e *Environment) GKEAutopilot() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraGKEEnableAutopilot, e.envDefault.ddInfra.gke.autopilot)
}

// Region returns the default region for the GCP environment
func (e *Environment) Region() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultRegionNameParamName, e.envDefault.gcp.region)
}

// Zone returns the default zone for the GCP environment
func (e *Environment) Zone() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultZoneNameParamName, e.envDefault.gcp.zone)
}

// OpenShiftPullSecretPath returns the path to the OpenShift pull secret file
func (e *Environment) OpenShiftPullSecretPath() string {
	return e.InfraConfig.Get(DDInfraOpenShiftPullSecretPath)
}

// EnableNestedVirtualization returns whether to enable nested virtualization
func (e *Environment) EnableNestedVirtualization() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraEnableNestedVirtualization, e.envDefault.ddInfra.openshift.nestedVirtualization)
}
