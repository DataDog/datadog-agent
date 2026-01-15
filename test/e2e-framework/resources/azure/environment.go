// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package azure

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	config "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	sdkazure "github.com/pulumi/pulumi-azure-native-sdk/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	azConfigNamespace = "azure-native"
	azNamerNamespace  = "az"

	// Azure Infra
	DDInfraDefaultSubscriptionID           = "az/defaultSubscriptionID"
	DDInfraDefaultContainerRegistry        = "az/defaultContainerRegistry"
	DDInfraDefaultResourceGroup            = "az/defaultResourceGroup"
	DDInfraDefaultVNetParamName            = "az/defaultVNet"
	DDInfraDefaultSubnetParamName          = "az/defaultSubnet"
	DDInfraDefaultSecurityGroupParamName   = "az/defaultSecurityGroup"
	DDInfraDefaultInstanceTypeParamName    = "az/defaultInstanceType"
	DDInfraDefaultARMInstanceTypeParamName = "az/defaultARMInstanceType"
	DDInfraDefaultPublicKeyPath            = "az/defaultPublicKeyPath"
	DDInfraDefaultPrivateKeyPath           = "az/defaultPrivateKeyPath"
	DDInfraDefaultPrivateKeyPassword       = "az/defaultPrivateKeyPassword"
	DDInfraAksLinuxKataNodeGroup           = "az/aks/linuxKataNodeGroup"
)

type Environment struct {
	*config.CommonEnvironment

	Namer namer.Namer

	envDefault environmentDefault
}

var _ config.Env = (*Environment)(nil)
var pulumiEnvVariables = []string{"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "ARM_CLIENT_ID", "ARM_CLIENT_SECRET"}

func NewEnvironment(ctx *pulumi.Context) (Environment, error) {
	env := Environment{
		Namer: namer.NewNamer(ctx, azNamerNamespace),
	}
	commonEnv, err := config.NewCommonEnvironment(ctx)
	if err != nil {
		return Environment{}, err
	}
	env.CommonEnvironment = &commonEnv
	env.envDefault = getEnvironmentDefault(config.FindEnvironmentName(commonEnv.InfraEnvironmentNames(), azNamerNamespace))

	// TODO: Remove this when we find a better way to automatically log in
	logIn(ctx, env.envDefault.azure.subscriptionID)

	azureProvider, err := sdkazure.NewProvider(ctx, string(config.ProviderAzure), &sdkazure.ProviderArgs{
		DisablePulumiPartnerId: pulumi.BoolPtr(true),
		SubscriptionId:         pulumi.StringPtr(env.envDefault.azure.subscriptionID),
		TenantId:               pulumi.StringPtr(env.envDefault.azure.tenantID),
		Location:               pulumi.StringPtr(env.envDefault.azure.location),
	})
	if err != nil {
		return Environment{}, err
	}
	env.RegisterProvider(config.ProviderAzure, azureProvider)

	return env, nil
}

// Cross Cloud Provider config
func (e *Environment) InternalRegistry() string {
	return "agentqa.azurecr.io"
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

func (e *Environment) DefaultSubscriptionID() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultSubscriptionID, e.envDefault.ddInfra.defaultSubscriptionID)
}
func (e *Environment) DefaultContainerRegistry() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultContainerRegistry, e.envDefault.ddInfra.defaultContainerRegistry)
}

func (e *Environment) DefaultResourceGroup() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultResourceGroup, e.envDefault.ddInfra.defaultResourceGroup)
}

func (e *Environment) DefaultVNet() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultVNetParamName, e.envDefault.ddInfra.defaultVNet)
}

func (e *Environment) DefaultSubnet() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultSubnetParamName, e.envDefault.ddInfra.defaultSubnet)
}

func (e *Environment) DefaultSecurityGroup() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultSecurityGroupParamName, e.envDefault.ddInfra.defaultSecurityGroup)
}

func (e *Environment) DefaultInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultInstanceTypeParamName, e.envDefault.ddInfra.defaultInstanceType)
}

func (e *Environment) DefaultARMInstanceType() string {
	return e.GetStringWithDefault(e.InfraConfig, DDInfraDefaultARMInstanceTypeParamName, e.envDefault.ddInfra.defaultARMInstanceType)
}

func (e *Environment) DefaultPublicKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPublicKeyPath)
}

func (e *Environment) DefaultPrivateKeyPath() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPath)
}

func (e *Environment) DefaultPrivateKeyPassword() string {
	return e.InfraConfig.Get(DDInfraDefaultPrivateKeyPassword)
}

func (e *Environment) GetCommonEnvironment() *config.CommonEnvironment {
	return e.CommonEnvironment
}

// LinuxKataNodeGroup Whether to deploy a kata node pool
func (e *Environment) LinuxKataNodeGroup() bool {
	return e.GetBoolWithDefault(e.InfraConfig, DDInfraAksLinuxKataNodeGroup, e.envDefault.ddInfra.aks.linuxKataNodeGroup)
}

func logIn(ctx *pulumi.Context, subscription string) {
	// Don't log in if the env variables are already set, used to avoid running `az login` in CI
	envVariablesSet := true
	for _, envVar := range pulumiEnvVariables {
		if os.Getenv(envVar) == "" {
			envVariablesSet = false
			break
		}
	}

	if envVariablesSet {
		return
	}

	cmd := exec.Command("az", "account", "show", "--subscription", subscription)
	shouldLogIn := false

	if err := cmd.Run(); err != nil {
		shouldLogIn = true
	} else {
		// Check the token is not expired
		cmd = exec.Command("az", "account", "get-access-token", "--query", "\"expiresOn\"", "--output", "tsv")
		out, err := cmd.Output()

		if err != nil {
			ctx.Log.Error(fmt.Sprintf("Error running `az account get-access-token`: %v", err), nil)
			shouldLogIn = true
		} else {
			tt, err := time.Parse(time.DateTime, strings.TrimSpace(string(out)))
			if err != nil {
				ctx.Log.Error(fmt.Sprintf("Error parsing the token expiration date`: %v", err), nil)
			} else {
				shouldLogIn = tt.Before(time.Now())
			}
		}
	}

	if shouldLogIn {
		if err := exec.Command("az", "login").Run(); err != nil {
			ctx.Log.Error(fmt.Sprintf("Error running `az login`: %v", err), nil)
		}
	}
}
