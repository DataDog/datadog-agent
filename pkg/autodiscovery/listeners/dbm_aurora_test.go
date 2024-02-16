// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	dbmconfig "github.com/DataDog/datadog-agent/pkg/databasemonitoring/config"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type mockRDSClientConfigurer func(k *aws.MockRDSClient)

func TestDBMAuroraListener(t *testing.T) {
	testCases := []struct {
		name                string
		config              dbmconfig.AutodiscoverClustersConfig
		rdsClientConfigurer mockRDSClientConfigurer
		previousServices    map[string]struct{}
		expectedServices    []*DBMAuroraService
		expectedDelServices []*DBMAuroraService
	}{
		{
			name: "single endpoint discovered and created",
			config: dbmconfig.AutodiscoverClustersConfig{
				DiscoveryInterval: 1,
				RoleArn:           "arn:aws:iam::123456789012:role/MyRole",
				Clusters: []dbmconfig.ClustersConfig{
					{
						Type:       dbmconfig.Postgres,
						Region:     "us-east-1",
						ClusterIds: []string{"my-cluster-1"},
					},
				},
			},
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClusterEndpoints([]string{"my-cluster-1"}).Return(
					map[string]*aws.AuroraCluster{
						"my-cluster-1": {
							Instances: []*aws.Instance{
								{
									Endpoint:   "my-endpoint",
									Port:       5432,
									Region:     "us-east-1",
									IamEnabled: true,
								},
							},
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMAuroraService{
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "19514be0f2d4837d",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
			},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "multiple endpoints discovered and created",
			config: dbmconfig.AutodiscoverClustersConfig{
				DiscoveryInterval: 1,
				RoleArn:           "arn:aws:iam::123456789012:role/MyRole",
				Clusters: []dbmconfig.ClustersConfig{
					{
						Type:       dbmconfig.Postgres,
						Region:     "us-east-1",
						ClusterIds: []string{"my-cluster-1"},
					},
				},
			},
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClusterEndpoints([]string{"my-cluster-1"}).Return(
					map[string]*aws.AuroraCluster{
						"my-cluster-1": {
							Instances: []*aws.Instance{
								{
									Endpoint:   "my-endpoint",
									Port:       5432,
									Region:     "us-east-1",
									IamEnabled: true,
								},
								{
									Endpoint:   "foo-endpoint",
									Port:       5432,
									Region:     "us-east-1",
									IamEnabled: true,
								},
								{
									Endpoint:   "bar-endpoint",
									Port:       5444,
									Region:     "us-east-1",
									IamEnabled: false,
								},
							},
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMAuroraService{
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "19514be0f2d4837d",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "9c140ca81a81f639",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "foo-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "26b65ecd56cd0a64",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "bar-endpoint",
						Port:       5444,
						Region:     "us-east-1",
						IamEnabled: false,
					},
				},
			},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "endpoints are deleted when no longer discovered",
			config: dbmconfig.AutodiscoverClustersConfig{
				DiscoveryInterval: 1,
				RoleArn:           "arn:aws:iam::123456789012:role/MyRole",
				Clusters: []dbmconfig.ClustersConfig{
					{
						Type:       dbmconfig.Postgres,
						Region:     "us-east-1",
						ClusterIds: []string{"my-cluster-1"},
					},
				},
			},
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				gomock.InOrder(
					k.EXPECT().GetAuroraClusterEndpoints([]string{"my-cluster-1"}).Return(
						map[string]*aws.AuroraCluster{
							"my-cluster-1": {
								Instances: []*aws.Instance{
									{
										Endpoint:   "my-endpoint",
										Port:       5432,
										Region:     "us-east-1",
										IamEnabled: true,
									},
									{
										Endpoint:   "foo-endpoint",
										Port:       5432,
										Region:     "us-east-1",
										IamEnabled: true,
									},
									{
										Endpoint:   "bar-endpoint",
										Port:       5444,
										Region:     "us-east-1",
										IamEnabled: false,
									},
								},
							},
						}, nil).Times(1),
					k.EXPECT().GetAuroraClusterEndpoints([]string{"my-cluster-1"}).Return(
						map[string]*aws.AuroraCluster{
							"my-cluster-1": {
								Instances: []*aws.Instance{
									{
										Endpoint:   "bar-endpoint",
										Port:       5444,
										Region:     "us-east-1",
										IamEnabled: false,
									},
								},
							},
						}, nil).Times(1),
				)
			},
			expectedServices: []*DBMAuroraService{
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "19514be0f2d4837d",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "9c140ca81a81f639",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "foo-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "26b65ecd56cd0a64",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "bar-endpoint",
						Port:       5444,
						Region:     "us-east-1",
						IamEnabled: false,
					},
				},
			},
			expectedDelServices: []*DBMAuroraService{
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "19514be0f2d4837d",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
				{
					adIdentifier: dbmAdIdentifier,
					entityID:     "9c140ca81a81f639",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					instance: &aws.Instance{
						Endpoint:   "foo-endpoint",
						Port:       5432,
						Region:     "us-east-1",
						IamEnabled: true,
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			newSvc := make(chan Service, 10)
			delSvc := make(chan Service, 10)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockConfig := config.Mock(t)
			mockConfig.SetWithoutSource("autodiscover_aurora_clusters", tc.config)
			mockClient := aws.NewMockRDSClient(ctrl)
			tc.rdsClientConfigurer(mockClient)
			// TODO: support multiple regions in my test setup
			awsClients := make(map[string]aws.RDSClient)
			for _, cluster := range tc.config.Clusters {
				awsClients[cluster.Region] = mockClient
			}
			ticks := make(chan time.Time, 1)
			l := newDBMAuroraListener(tc.config, tc.previousServices, awsClients, ticks)
			l.Listen(newSvc, delSvc)
			// ensure loop executes at least once
			ticks <- time.Now()

			// shutdown service, and get output from channels
			l.Stop()
			close(newSvc)
			close(delSvc)

			// assert that the expected new services were created
			createdServices := make([]*DBMAuroraService, 0)
			for newService := range newSvc {
				dbmAuroraService, ok := newService.(*DBMAuroraService)
				if !ok {
					require.Fail(t, "received service of incorrect type on service chan")
				}
				createdServices = append(createdServices, dbmAuroraService)
			}
			assert.Equal(t, len(tc.expectedServices), len(createdServices))
			assert.ElementsMatch(t, tc.expectedServices, createdServices)

			// assert that the expected deleted services were created
			deletedServices := make([]*DBMAuroraService, 0)
			for delService := range delSvc {
				dbmAuroraService, ok := delService.(*DBMAuroraService)
				if !ok {
					require.Fail(t, "received service of incorrect type on service chan")
				}
				deletedServices = append(deletedServices, dbmAuroraService)
			}
			assert.Equal(t, len(tc.expectedDelServices), len(deletedServices))
			assert.ElementsMatch(t, tc.expectedDelServices, deletedServices)
		})
	}
}
