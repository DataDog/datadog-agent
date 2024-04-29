// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package listeners

import (
	"context"
	"errors"
	"fmt"
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

const defaultClusterTag = "datadoghq.com/scrape:true"

func TestDBMAuroraListener(t *testing.T) {
	testCases := []struct {
		name                  string
		config                dbmconfig.AuroraConfig
		numDiscoveryIntervals int
		rdsClientConfigurer   mockRDSClientConfigurer
		expectedServices      []*DBMAuroraService
		expectedDelServices   []*DBMAuroraService
	}{
		{
			name: "GetAuroraClustersFromTags context deadline exceeded produces no services",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				QueryTimeout:      1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClustersFromTags(contextWithTimeout(1*time.Second), []string{defaultClusterTag}).DoAndReturn(
					func(ctx context.Context, tags []string) ([]string, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					}).AnyTimes()
			},
			expectedServices:    []*DBMAuroraService{},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "GetAuroraClusterEndpoints context deadline exceeded produces no services",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				QueryTimeout:      1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				gomock.InOrder(
					k.EXPECT().GetAuroraClustersFromTags(gomock.Any(), []string{defaultClusterTag}).Return([]string{"my-cluster-1"}, nil).AnyTimes(),
					k.EXPECT().GetAuroraClusterEndpoints(contextWithTimeout(1*time.Second), []string{"my-cluster-1"}).DoAndReturn(
						func(ctx context.Context, ids []string) (map[string]*aws.AuroraCluster, error) {
							<-ctx.Done()
							return nil, ctx.Err()
						}).AnyTimes(),
				)

			},
			expectedServices:    []*DBMAuroraService{},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "GetAuroraClustersFromTags error produces no services",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClustersFromTags(gomock.Any(), []string{defaultClusterTag}).Return(nil, errors.New("big bad error")).AnyTimes()
			},
			expectedServices:    []*DBMAuroraService{},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "GetAuroraClusterEndpoints error produces no services",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				gomock.InOrder(
					k.EXPECT().GetAuroraClustersFromTags(gomock.Any(), []string{defaultClusterTag}).Return([]string{"my-cluster-1"}, nil).AnyTimes(),
					k.EXPECT().GetAuroraClusterEndpoints(gomock.Any(), []string{"my-cluster-1"}).Return(nil, errors.New("big bad error")).AnyTimes(),
				)
			},
			expectedServices:    []*DBMAuroraService{},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "single endpoint discovered and created",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 1,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClustersFromTags(gomock.Any(), []string{defaultClusterTag}).Return([]string{"my-cluster-1"}, nil).AnyTimes()
				k.EXPECT().GetAuroraClusterEndpoints(gomock.Any(), []string{"my-cluster-1"}).Return(
					map[string]*aws.AuroraCluster{
						"my-cluster-1": {
							Instances: []*aws.Instance{
								{
									Endpoint:   "my-endpoint",
									Port:       5432,
									IamEnabled: true,
									Engine:     "aurora-postgresql",
								},
							},
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMAuroraService{
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "f7fee36c58e3da8a",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					region:       "us-east-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						IamEnabled: true,
						Engine:     "aurora-postgresql",
					},
				},
			},
			expectedDelServices: []*DBMAuroraService{},
		},
		{
			name: "multiple endpoints discovered from single cluster and created",
			config: dbmconfig.AuroraConfig{
				DiscoveryInterval: 1,
				Region:            "us-east-1",
				Tags:              []string{defaultClusterTag},
			},
			numDiscoveryIntervals: 1,
			rdsClientConfigurer: func(k *aws.MockRDSClient) {
				k.EXPECT().GetAuroraClustersFromTags(gomock.Any(), []string{defaultClusterTag}).Return([]string{"my-cluster-1"}, nil).AnyTimes()
				k.EXPECT().GetAuroraClusterEndpoints(gomock.Any(), []string{"my-cluster-1"}).Return(
					map[string]*aws.AuroraCluster{
						"my-cluster-1": {
							Instances: []*aws.Instance{
								{
									Endpoint:   "my-endpoint",
									Port:       5432,
									IamEnabled: true,
									Engine:     "aurora-postgresql",
								},
								{
									Endpoint:   "foo-endpoint",
									Port:       5432,
									IamEnabled: true,
									Engine:     "aurora-postgresql",
								},
								{
									Endpoint:   "bar-endpoint",
									Port:       5444,
									IamEnabled: false,
									Engine:     "aurora-postgresql",
								},
							},
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMAuroraService{
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "f7fee36c58e3da8a",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					region:       "us-east-1",
					instance: &aws.Instance{
						Endpoint:   "my-endpoint",
						Port:       5432,
						IamEnabled: true,
						Engine:     "aurora-postgresql",
					},
				},
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "509dbfd2cc1ae2be",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					region:       "us-east-1",
					instance: &aws.Instance{
						Endpoint:   "foo-endpoint",
						Port:       5432,
						IamEnabled: true,
						Engine:     "aurora-postgresql",
					},
				},
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "cc92e57c9b7b7531",
					checkName:    "postgres",
					clusterID:    "my-cluster-1",
					region:       "us-east-1",
					instance: &aws.Instance{
						Endpoint:   "bar-endpoint",
						Port:       5444,
						IamEnabled: false,
						Engine:     "aurora-postgresql",
					},
				},
			},
			expectedDelServices: []*DBMAuroraService{},
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
			mockAWSClient := aws.NewMockRDSClient(ctrl)
			tc.rdsClientConfigurer(mockAWSClient)
			ticks := make(chan time.Time, 1)
			l := newDBMAuroraListener(tc.config, mockAWSClient, ticks)
			l.Listen(newSvc, delSvc)
			// execute loop
			for i := 0; i < tc.numDiscoveryIntervals; i++ {
				ticks <- time.Now()
			}

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

func contextWithTimeout(t time.Duration) gomock.Matcher {
	return contextWithTimeoutMatcher{
		timeout: t,
	}
}

type contextWithTimeoutMatcher struct {
	timeout time.Duration
}

func (m contextWithTimeoutMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return false
	}

	delta := time.Until(deadline) - m.timeout
	return delta < time.Millisecond*50
}

func (m contextWithTimeoutMatcher) String() string {
	return fmt.Sprintf("have a deadline from a timeout of %d milliseconds", m.timeout.Milliseconds())
}
