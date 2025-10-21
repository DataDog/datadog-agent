package helm

import (
	"github.com/DataDog/test-infra-definitions/common/config"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type InstallArgs struct {
	RepoURL     string
	ChartName   string
	InstallName string
	Namespace   string
	ValuesYAML  pulumi.AssetOrArchiveArrayInput
	Values      pulumi.MapInput
	Version     pulumi.StringPtrInput
}

// Important: set relevant Kubernetes provider in `opts`
func NewInstallation(e config.Env, args InstallArgs, opts ...pulumi.ResourceOption) (*helm.Release, error) {
	releaseArgs := &helm.ReleaseArgs{
		Namespace: pulumi.StringPtr(args.Namespace),
		Name:      pulumi.StringPtr(args.InstallName),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: pulumi.StringPtr(args.RepoURL),
		},
		Chart:            pulumi.String(args.ChartName),
		CreateNamespace:  pulumi.BoolPtr(true),
		DependencyUpdate: pulumi.BoolPtr(true),
		ValueYamlFiles:   args.ValuesYAML,
		Values:           args.Values,
		Version:          args.Version,
	}
	// Only set timeout for OpenShift deployments
	if args.Namespace == "datadog-openshift" {
		releaseArgs.Timeout = pulumi.IntPtr(600)
	}
	return helm.NewRelease(e.Ctx(), args.InstallName, releaseArgs, opts...)
}
