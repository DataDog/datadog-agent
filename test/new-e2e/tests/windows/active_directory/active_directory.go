package active_directory

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/namer"
	"github.com/DataDog/test-infra-definitions/common/utils"
	infraComponents "github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	provisionerBaseID = "aws-ec2vm-active-directory-"
	defaultVMName     = "dcvm"
)

type ActiveDirectoryEnv struct {
	DomainControllerHost *components.RemoteHost
	DomainController     *RemoteActiveDirectory
	FakeIntake           *components.FakeIntake
}

// DomainUser represents an Active Directory user
type DomainUser struct {
	Username string
	Password string
}

type Params struct {
	DomainName      string
	DomainPassword  string
	DomainUsers     []DomainUser
	ResourceOptions []pulumi.ResourceOption
}
type Option = func(*Params) error

func WithDomainName(domainName string) func(*Params) error {
	return func(p *Params) error {
		p.DomainName = domainName
		return nil
	}
}

func WithDomainPassword(domainPassword string) func(*Params) error {
	return func(p *Params) error {
		p.DomainPassword = domainPassword
		return nil
	}
}

func WithDomainUser(username, password string) func(params *Params) error {
	return func(p *Params) error {
		p.DomainUsers = append(p.DomainUsers, DomainUser{
			Username: username,
			Password: password,
		})
		return nil
	}
}

func NewParams(options ...Option) (*Params, error) {
	p := &Params{
		// JL: Should we set sensible defaults here ?
	}
	return common.ApplyOption(p, options)
}

type ProvisionerParams struct {
	name string

	activeDirectoryOptions []Option
	fakeintakeOptions      []fakeintake.Option
}

func newProvisionerParams() *ProvisionerParams {
	// We use nil arrays to decide if we should create or not
	return &ProvisionerParams{
		name:              defaultVMName,
		fakeintakeOptions: []fakeintake.Option{},
	}
}

// ProvisionerOption is a provisioner option.
type ProvisionerOption func(*ProvisionerParams) error

// WithName sets the name of the provisioner.
func WithName(name string) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithActiveDirectoryOptions adds options to Active Directory.
func WithActiveDirectoryOptions(opts ...Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.activeDirectoryOptions = append(params.activeDirectoryOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions adds options to the FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintake.Option) ProvisionerOption {
	return func(params *ProvisionerParams) error {
		params.fakeintakeOptions = append(params.fakeintakeOptions, opts...)
		return nil
	}
}

// RemoteActiveDirectory represents an Active Directory domain setup on a remote host
type RemoteActiveDirectory struct {
	ActiveDirectoryOutput
}

type ActiveDirectoryOutput struct {
	infraComponents.JSONImporter
}

// ActiveDirectory is an Active Directory domain environment
type ActiveDirectory struct {
	pulumi.ResourceState
	infraComponents.Component
	namer namer.Namer
	host  *remote.Host
}

func (dc *ActiveDirectory) Export(ctx *pulumi.Context, out *ActiveDirectoryOutput) error {
	return infraComponents.Export(ctx, dc, out)
}

// NewActiveDirectory creates a new instance of an Active Directory domain deployment
func NewActiveDirectory(ctx *pulumi.Context, e *config.CommonEnvironment, host *remote.Host, option ...Option) (*ActiveDirectory, error) {
	params, paramsErr := NewParams(option...)
	if paramsErr != nil {
		return nil, paramsErr
	}

	domainControllerComp, err := infraComponents.NewComponent(*e, host.Name(), func(comp *ActiveDirectory) error {
		comp.namer = e.CommonNamer.WithPrefix(comp.Name())
		comp.host = host

		resourceOptions := []pulumi.ResourceOption{
			pulumi.Parent(comp),
		}

		installForestCmd, err := host.OS.Runner().Command(comp.namer.ResourceName("install-forest"), &command.Args{
			Create: pulumi.String(windows.PsHost().
				AddActiveDirectoryDomainServicesWindowsFeature().
				ImportActiveDirectoryDomainServicesModule().
				InstallADDSForest(params.DomainName, params.DomainPassword).
				Compile()),
			// JL: I hesitated to provide a Delete function but Uninstall-ADDSDomainController looks
			// non-trivial to call, and I couldn't test it.
		}, resourceOptions...)
		if err != nil {
			return err
		}
		resourceOptions = utils.MergeOptions(resourceOptions, utils.PulumiDependsOn(installForestCmd))

		ensureAdwsStartedCmd, err := host.OS.Runner().Command(comp.namer.ResourceName("ensure-adws-started"), &command.Args{
			Create: pulumi.String("while (1) { try { (Get-Service ADWS -ErrorAction SilentlyContinue).WaitForStatus('Running', '00:01:00'); break; } catch { Write-Host 'Not yet ready'; Start-Sleep -Seconds 10 } }"),
		}, resourceOptions...)
		if err != nil {
			return err
		}
		resourceOptions = utils.MergeOptions(resourceOptions, utils.PulumiDependsOn(ensureAdwsStartedCmd))

		if len(params.DomainUsers) > 0 {
			cmdHost := windows.PsHost()
			for _, user := range params.DomainUsers {
				cmdHost.AddActiveDirectoryUser(user.Username, user.Password)
			}
			_, err := host.OS.Runner().Command(comp.namer.ResourceName("ensure-adws-started"), &command.Args{
				Create: pulumi.String(cmdHost.Compile()),
			}, resourceOptions...)
			if err != nil {
				return err
			}
		}

		return nil
	}, pulumi.Parent(host), pulumi.DeletedWith(host))
	if err != nil {
		return nil, err
	}

	return domainControllerComp, nil
}

// Provisioner creates an Active Directory environment on a given VM.
func Provisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[ActiveDirectoryEnv] {
	params := newProvisionerParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply ProvisionerOption, err: %w", err))
	}

	return e2e.NewTypedPulumiProvisioner[ActiveDirectoryEnv](provisionerBaseID+params.name, func(ctx *pulumi.Context, env *ActiveDirectoryEnv) error {
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		vm, err := ec2.NewVM(awsEnv, params.name, ec2.WithOS(os.WindowsDefault))
		if err != nil {
			return err
		}
		vm.Export(ctx, &env.DomainControllerHost.HostOutput)

		domainController, err := NewActiveDirectory(ctx, awsEnv.CommonEnvironment, vm, params.activeDirectoryOptions...)
		if err != nil {
			return err
		}
		domainController.Export(ctx, &env.DomainController.ActiveDirectoryOutput)

		// JL: can params.fakeintakeOptions be nil, and how should we handle it?
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, params.name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		return nil
	}, nil)
}
