// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGetAuroraClusterEndpoints(t *testing.T) {
	testCases := []struct {
		name                           string
		configureClient                mockrdsServiceConfigurer
		clusterIDs                     []string
		dbmTag                         *string
		expectedAuroraClusterEndpoints map[string]*AuroraCluster
		expectedErr                    error
	}{
		{
			name:            "no cluster ids given",
			configureClient: func(*MockrdsService) {},
			clusterIDs:      nil,
			expectedErr:     errors.New("at least one database cluster identifier is required"),
		},
		{
			name: "single cluster id returns no results from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{}, nil).Times(1)
			},
			clusterIDs:                     []string{"test-cluster"},
			expectedAuroraClusterEndpoints: nil,
			expectedErr:                    errors.New("no endpoints found for aurora clusters with id(s): test-cluster"),
		},
		{
			name: "single cluster id returns error response from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(nil, errors.New("big time error")).Times(1)
			},
			clusterIDs:                     []string{"test-cluster"},
			expectedAuroraClusterEndpoints: nil,
			expectedErr:                    errors.New("error running GetAuroraClusterEndpoints big time error"),
		},
		{
			name: "single cluster id returns single endpoint from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns single endpoint from API with custom DBM tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							TagList:                          []types.Tag{{Key: aws.String("datadoghq.com/dbm"), Value: aws.String("custom")}},
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			dbmTag:     aws.String("datadoghq.com/dbm:custom"),
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
							DbmEnabled: true,
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns single endpoint from API with custom bare DBM tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							TagList:                          []types.Tag{{Key: aws.String("datadoghq.com/dbm")}},
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			dbmTag:     aws.String("datadoghq.com/dbm"),
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
							DbmEnabled: true,
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns many endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int32(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint-2",
							Port:       5432,
							IamEnabled: false,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint-3",
							Port:       5444,
							IamEnabled: false,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
			},
		},
		{
			name: "single cluster id returns some unavailable endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("terminating"),
							Engine:                           aws.String("aurora-postgresql"),
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int32(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("terminating"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
			},
		},
		{
			name: "multiple cluster ids returns single endpoint from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), createDescribeDBInstancesRequest([]string{"test-cluster"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
				k.EXPECT().DescribeDBInstances(gomock.Any(), createDescribeDBInstancesRequest([]string{"test-cluster-2"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster", "test-cluster-2"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
			},
		},
		{
			name: "multiple cluster ids returns many endpoints from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), createDescribeDBInstancesRequest([]string{"test-cluster"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							TagList:                          []types.Tag{{Key: aws.String("datadoghq.com/dbm"), Value: aws.String("true")}},
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(false),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)

				k.EXPECT().DescribeDBInstances(gomock.Any(), createDescribeDBInstancesRequest([]string{"test-cluster-2"})).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-3"),
								Port:    aws.Int32(5444),
							},
							DBClusterIdentifier:              aws.String("test-cluster-2"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1c"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster", "test-cluster-2"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
							DbmEnabled: true,
						},
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint-2",
							Port:       5432,
							IamEnabled: false,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
				"test-cluster-2": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster-2",
							Endpoint:   "test-endpoint-3",
							Port:       5444,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
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
			dbmTag := defaultDbmTag
			if tt.dbmTag != nil {
				dbmTag = *tt.dbmTag
			}
			clusters, err := client.GetAuroraClusterEndpoints(context.Background(), tt.clusterIDs, dbmTag)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAuroraClusterEndpoints, clusters)
		})
	}
}

func TestGetAuroraClusterEndpointsDbName(t *testing.T) {
	testCases := []struct {
		name                           string
		configureClient                mockrdsServiceConfigurer
		clusterIDs                     []string
		expectedAuroraClusterEndpoints map[string]*AuroraCluster
		expectedErr                    error
	}{
		{
			name: "aurora-postgresql engine returns default postgres db name",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "postgres",
						},
					},
				},
			},
		},
		{
			name: "aurora-mysql engine returns default db name for engine",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-mysql"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-mysql",
							DbName:     "mysql",
						},
					},
				},
			},
		},
		{
			name: "aurora-postgresql engine returns default db name for engine",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							DBName:                           aws.String("test-db-name"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs: []string{"test-cluster"},
			expectedAuroraClusterEndpoints: map[string]*AuroraCluster{
				"test-cluster": {
					Instances: []*Instance{
						{
							ClusterID:  "test-cluster",
							Endpoint:   "test-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "aurora-postgresql",
							DbName:     "test-db-name",
						},
					},
				},
			},
		},
		{
			name: "missing engine returns error",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs:  []string{"test-cluster"},
			expectedErr: errors.New("no endpoints found for aurora clusters with id(s): test-cluster"),
		},
		{
			name: "unsupported engine returns error",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), gomock.Any()).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBClusterIdentifier:              aws.String("test-cluster"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("does-not-exist"),
						},
					},
				}, nil).Times(1)
			},
			clusterIDs:  []string{"test-cluster"},
			expectedErr: errors.New("no endpoints found for aurora clusters with id(s): test-cluster"),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := NewMockrdsService(ctrl)
			tt.configureClient(mockClient)
			client := &Client{client: mockClient}
			clusters, err := client.GetAuroraClusterEndpoints(context.Background(), tt.clusterIDs, defaultDbmTag)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAuroraClusterEndpoints, clusters)
		})
	}
}

func TestGetAuroraClustersFromTags(t *testing.T) {
	testCases := []struct {
		name               string
		configureClient    mockrdsServiceConfigurer
		tags               []string
		expectedClusterIDs []string
		expectedErr        error
	}{
		{
			name: "empty tag filter returns all clusters",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{},
			expectedClusterIDs: []string{"test-cluster"},
		},
		{
			name: "single tag filter returns error from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(nil, errors.New("big time error")).Times(1)
			},
			tags:        []string{"test:tag"},
			expectedErr: errors.New("error running GetAuroraClustersFromTags: big time error"),
		},
		{
			name: "single tag filter returns no results from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{}, nil).Times(1)
			},
			tags:               []string{"test:tag"},
			expectedClusterIDs: []string{},
		},
		{
			name: "single tag filter returns single result from API with matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag"},
			expectedClusterIDs: []string{"test-cluster"},
		},
		{
			name: "single tag filter returns single result from API with non-matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag2"},
			expectedClusterIDs: []string{},
		},
		{
			name: "single tag filter returns multiple results from API with matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
						{
							DBClusterIdentifier: aws.String("test-cluster-2"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag"},
			expectedClusterIDs: []string{"test-cluster", "test-cluster-2"},
		},
		{
			name: "single tag filter returns multiple results from API, one cluster doesn't match tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
						{
							DBClusterIdentifier: aws.String("test-cluster-2"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag2"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag"},
			expectedClusterIDs: []string{"test-cluster"},
		},
		{
			name: "single tag filter returns multiple results from API, one cluster has no tags",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList:             nil,
						},
						{
							DBClusterIdentifier: aws.String("test-cluster-2"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag"},
			expectedClusterIDs: []string{"test-cluster-2"},
		},
		{
			name: "multiple tag filter returns multiple results from API all matching",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
								{
									Key:   aws.String("test2"),
									Value: aws.String("tag2"),
								},
							},
						},
						{
							DBClusterIdentifier: aws.String("test-cluster-2"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
								{
									Key:   aws.String("test2"),
									Value: aws.String("tag2"),
								},
								{
									Key:   aws.String("foo"),
									Value: aws.String("bar"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag", "test2:tag2"},
			expectedClusterIDs: []string{"test-cluster", "test-cluster-2"},
		},
		{
			name: "multiple pages returns ids from all pages",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					Marker: aws.String("next"),
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
								{
									Key:   aws.String("test2"),
									Value: aws.String("tag2"),
								},
							},
						},
					},
				}, nil).Times(1)
				k.EXPECT().DescribeDBClusters(gomock.Any(), &rds.DescribeDBClustersInput{
					Marker: aws.String("next"),
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBClustersOutput{
					DBClusters: []types.DBCluster{
						{
							DBClusterIdentifier: aws.String("test-cluster-2"),
							TagList: []types.Tag{
								{
									Key:   aws.String("test"),
									Value: aws.String("tag"),
								},
								{
									Key:   aws.String("test2"),
									Value: aws.String("tag2"),
								},
								{
									Key:   aws.String("foo"),
									Value: aws.String("bar"),
								},
							},
						},
					},
				}, nil).Times(1)
			},
			tags:               []string{"test:tag", "test2:tag2"},
			expectedClusterIDs: []string{"test-cluster", "test-cluster-2"},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockClient := NewMockrdsService(ctrl)
			tt.configureClient(mockClient)
			client := &Client{client: mockClient}
			clusters, err := client.GetAuroraClustersFromTags(context.Background(), tt.tags)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedClusterIDs, clusters)
		})
	}
}
