package hyperv

import (
	config "github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	hvConfigNamespace = "hyperv"
	hvNamerNamespace  = "hv"

	// HyperV Infra (local)
	DDInfraDefaultPublicKeyPath      = "hv/defaultPublicKeyPath"
	DDInfraDefaultPrivateKeyPath     = "hv/defaultPrivateKeyPath"
	DDInfraDefaultPrivateKeyPassword = "hv/defaultPrivateKeyPassword"
)

type Environment struct {
	*config.CommonEnvironment

	Namer namer.Namer
}

var _ config.Env = (*Environment)(nil)

func NewEnvironment(ctx *pulumi.Context) (Environment, error) {
	env := Environment{
		Namer: namer.NewNamer(ctx, hvNamerNamespace),
	}

	commonEnv, err := config.NewCommonEnvironment(ctx)
	if err != nil {
		return Environment{}, err
	}

	env.CommonEnvironment = &commonEnv

	return env, nil
}

// Cross Cloud Provider config
func (e *Environment) InternalRegistry() string {
	return "none"
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

func (e *Environment) GetCommonEnvironment() *config.CommonEnvironment {
	return e.CommonEnvironment
}

// We need to implement unrelated fonctions because of current OS design
// to implement common.Environment interface
func (e *Environment) DefaultInstanceType() string {
	panic("not implemented")
}

func (e *Environment) DefaultARMInstanceType() string {
	panic("not implemented")
}
