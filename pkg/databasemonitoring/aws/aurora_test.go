// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package aws

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"testing"
)

type mockrdsServiceConfigurer func(k *MockrdsService)

func TestGetAuroraClusterEndpoints(t *testing.T) {
	testCases := []struct {
		name                           string
		configureClient                mockrdsServiceConfigurer
		clusterIds                     []string
		expectedAuroraClusterEndpoints map[string]*AuroraCluster
		expectedErr                    error
	}{
		{
			name:            "no cluster ids given",
			configureClient: func(k *MockrdsService) {},
			clusterIds:      nil,
			expectedErr:     errors.New("at least one database cluster identifier is required"),
		},
		{
			name: "single cluster id returns no results from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any()).Return(&rds.DescribeDBInstancesOutput{}, nil).Times(1)
			},
			clusterIds:                     []string{"test-cluster"},
			expectedAuroraClusterEndpoints: nil,
			expectedErr:                    errors.New("no endpoints found for aurora clusters with id(s): test-cluster"),
		},
		{
			name: "single cluster id returns error response from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any()).Return(nil, errors.New("big time error")).Times(1)
			},
			clusterIds:                     []string{"test-cluster"},
			expectedAuroraClusterEndpoints: nil,
			expectedErr:                    errors.New("error describing aurora DB clusters: big time error"),
		},
		{
			name: "single cluster id returns single endpoint from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []*rds.DBInstance{
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
					},
				}, nil).Times(1)
			},
			clusterIds: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns many endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []*rds.DBInstance{
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int64(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
					},
				}, nil).Times(1)
			},
			clusterIds: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
						},
						{
							Endpoint:   "test-endpoint-2",
							Port:       5432,
							IamEnabled: false,
						},
						{
							Endpoint:   "test-endpoint-3",
							Port:       5444,
							IamEnabled: false,
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns some unavailable endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []*rds.DBInstance{
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("terminating"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int64(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("terminating"),
						},
					},
				}, nil).Times(1)
			},
			clusterIds: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
						},
					},
				},
			},
		},
		{
			name: "multiple cluster ids returns single endpoint from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(createDescribeDBInstancesRequest([]string{"test-cluster", "test-cluster-2"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []*rds.DBInstance{
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
					},
				}, nil).Times(1)
			},
			clusterIds: []string{"test-cluster", "test-cluster-2"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
						},
					},
				},
			},
		},
		{
			name: "multiple cluster ids returns many endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(createDescribeDBInstancesRequest([]string{"test-cluster", "test-cluster-2"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []*rds.DBInstance{
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int64(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
						{
							Endpoint: &rds.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int64(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster-2"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1c"),
							DBInstanceStatus:                 aws.String("available"),
						},
					},
				}, nil).Times(1)
			},
			clusterIds: []string{"test-cluster", "test-cluster-2"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
						},
						{
							Endpoint:   "test-endpoint-2",
							Port:       5432,
							IamEnabled: false,
						},
					},
				},
				"test-cluster-2": {
					Instances: []*Instance{
						{
							Endpoint:   "test-endpoint-3",
							Port:       5444,
							IamEnabled: true,
						},
					},
				},
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := NewMockrdsService(ctrl)
			tt.configureClient(mockClient)
			client := &Client{client: mockClient}
			clusters, err := client.GetAuroraClusterEndpoints(tt.clusterIds)
			if tt.expectedErr != nil {
				require.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedAuroraClusterEndpoints, clusters)
		})
	}
}

func createDescribeDBInstancesRequest(clusterIds []string) *rds.DescribeDBInstancesInput {
	idVals := make([]*string, 0)
	for _, id := range clusterIds {
		idVals = append(idVals, aws.String(id))
	}
	return &rds.DescribeDBInstancesInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-cluster-id"),
				Values: idVals,
			},
		},
	}
}
