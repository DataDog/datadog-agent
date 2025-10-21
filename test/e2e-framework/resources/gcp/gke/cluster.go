package gke

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewCluster(e gcp.Environment, name string, autopilot bool, opts ...pulumi.ResourceOption) (*container.Cluster, pulumi.StringOutput, error) {
	opts = append(opts, e.WithProviders(config.ProviderGCP))

	cluster, err := container.NewCluster(e.Ctx(), e.Namer.ResourceName(name), &container.ClusterArgs{
		Network:            pulumi.String(e.DefaultNetworkName()),
		Subnetwork:         pulumi.String(e.DefaultSubnet()),
		InitialNodeCount:   pulumi.Int(1),
		MinMasterVersion:   pulumi.String(e.KubernetesVersion()),
		NodeVersion:        pulumi.String(e.KubernetesVersion()),
		DeletionProtection: pulumi.Bool(false),
		EnableAutopilot:    pulumi.Bool(autopilot),
		NodeLocations:      pulumi.StringArray{pulumi.String(e.Zone())},
		PrivateClusterConfig: &container.ClusterPrivateClusterConfigArgs{
			EnablePrivateNodes:        pulumi.Bool(true),
			EnablePrivateEndpoint:     pulumi.Bool(true),
			PrivateEndpointSubnetwork: pulumi.String(e.DefaultSubnet()),
		},
		MasterAuthorizedNetworksConfig: &container.ClusterMasterAuthorizedNetworksConfigArgs{
			CidrBlocks: container.ClusterMasterAuthorizedNetworksConfigCidrBlockArray{
				&container.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs{
					CidrBlock:   pulumi.String("10.0.0.0/8"),
					DisplayName: pulumi.String("all private ips"),
				},
				&container.ClusterMasterAuthorizedNetworksConfigCidrBlockArgs{
					CidrBlock:   pulumi.String("172.16.0.0/12"),
					DisplayName: pulumi.String("ddbuild vpn private ips"),
				},
			}, // Empty array to disable master authorized networks
		},
		NodeConfig: &container.ClusterNodeConfigArgs{
			MachineType: pulumi.String(e.DefaultInstanceType()),

			OauthScopes: pulumi.StringArray{
				pulumi.String("https://www.googleapis.com/auth/compute"),
				pulumi.String("https://www.googleapis.com/auth/devstorage.read_only"),
				pulumi.String("https://www.googleapis.com/auth/logging.write"),
				pulumi.String("https://www.googleapis.com/auth/monitoring"),
			},
		},
	}, opts...)
	if err != nil {
		return nil, pulumi.StringOutput{}, err
	}
	// https://github.com/pulumi/examples/blob/master/gcp-go-gke/main.go
	kubeConfig := generateKubeconfig(cluster.Endpoint, cluster.Name, cluster.MasterAuth)
	return cluster, kubeConfig, nil
}

func generateKubeconfig(clusterEndpoint pulumi.StringOutput, clusterName pulumi.StringOutput,
	clusterMasterAuth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	context := pulumi.Sprintf("demo_%s", clusterName)

	return pulumi.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://%s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: gke-gcloud-auth-plugin
      installHint: Install gke-gcloud-auth-plugin for use with kubectl by following
        https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke
      provideClusterInfo: true
`,
		clusterMasterAuth.ClusterCaCertificate().Elem(),
		clusterEndpoint, context, context, context, context, context, context)
}
