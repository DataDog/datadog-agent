package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/integrations"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDBMAuroraListener(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	autodiscoveryConfig := integrations.AutodiscoverClustersConfig{
		DiscoveryInterval: 1,
		RoleArn:           "arn:aws:iam::123456789012:role/MyRole",
		Clusters: []integrations.ClustersConfig{
			{
				Type:       integrations.Postgres,
				Region:     "us-west-2",
				ClusterIds: []string{"my-cluster-1", "my-cluster-2"},
			},
		},
	}

	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("autodiscover_aurora_clusters", autodiscoveryConfig)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := aws.NewMockRDSClient(ctrl)
	auroraClusterOne := &aws.AuroraCluster{
		Instances: []aws.Instance{
			{
				Endpoint:   "my-endpoint",
				Port:       5432,
				Region:     "us-west-2",
				IamEnabled: true,
			},
		},
	}
	auroraClusterTwo := &aws.AuroraCluster{
		Instances: []aws.Instance{
			{
				Endpoint:   "foo-endpoint",
				Port:       53455,
				Region:     "us-west-2",
				IamEnabled: true,
			},
			{
				Endpoint:   "bar-endpoint",
				Port:       53455,
				Region:     "us-west-2",
				IamEnabled: true,
			},
		},
	}
	mockClient.EXPECT().GetAuroraClusterEndpoints([]string{"my-cluster-1", "my-cluster-2"}).Return(
		map[string]*aws.AuroraCluster{
			"my-cluster-1": auroraClusterOne,
			"my-cluster-2": auroraClusterTwo,
		}, nil).AnyTimes()
	awsClients := map[string]aws.RDSClient{
		"us-west-2": mockClient,
	}
	l, err := newDBMAuroraListener(&config.Listeners{}, awsClients)
	assert.NoError(t, err)
	l.Listen(newSvc, delSvc)
	l.Stop()
}

// TODO: use this for table tests
type mockAWSClientConfigurer func(k *aws.MockRDSClient)
