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

func TestGetRdsInstancesFromTags(t *testing.T) {
	testCases := []struct {
		name              string
		configureClient   mockrdsServiceConfigurer
		tags              []string
		expectedInstances []Instance
		expectedErr       error
	}{
		{
			name: "not tag filter returns all results from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{},
			expectedInstances: []Instance{{
				ID:         "test-instance",
				Endpoint:   "test-endpoint",
				Port:       5432,
				IamEnabled: true,
				Engine:     "postgres",
				DbmEnabled: false,
				DbName:     "postgres",
			}},
		},
		{
			name: "single tag filter returns error from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(nil, errors.New("big time error")).Times(1)
			},
			tags:        []string{"test:tag"},
			expectedErr: errors.New("error running GetRdsInstancesFromTags: big time error"),
		},
		{
			name: "single tag filter returns no results from API",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{}, nil).Times(1)
			},
			tags:              []string{"test:tag"},
			expectedInstances: []Instance{},
		},
		{
			name: "single tag filter returns single result from API with matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag"},
			expectedInstances: []Instance{{
				ID:         "test-instance",
				Endpoint:   "test-endpoint",
				Port:       5432,
				IamEnabled: true,
				Engine:     "postgres",
				DbmEnabled: false,
				DbName:     "postgres",
			}},
		},
		{
			name: "single tag filter returns single result from API with non-matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
			},
			tags:              []string{"test:tag2"},
			expectedInstances: []Instance{},
		},
		{
			name: "single tag filter returns multiple results from API with matching tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance-2"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag"},
			expectedInstances: []Instance{
				{
					ID:         "test-instance",
					Endpoint:   "test-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
				}, {
					ID:         "test-instance-2",
					Endpoint:   "test-endpoint-2",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
				},
			},
		},
		{
			name: "single tag filter returns multiple results from API, one instance doesn't match tag",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag2")}},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag"},
			expectedInstances: []Instance{
				{
					ID:         "test-instance",
					Endpoint:   "test-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
				}},
		},
		{
			name: "single tag filter returns multiple results from API, one instance has no tags",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag"},
			expectedInstances: []Instance{
				{
					ID:         "test-instance",
					Endpoint:   "test-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
				}},
		},
		{
			name: "multiple tag filter returns multiple results from API all matching",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList: []types.Tag{
								{Key: aws.String("test"), Value: aws.String("tag")},
								{Key: aws.String("test"), Value: aws.String("tag2")},
								{Key: aws.String("test"), Value: aws.String("tag3")},
							},
						},
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance-2"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							TagList: []types.Tag{
								{Key: aws.String("test"), Value: aws.String("tag")},
								{Key: aws.String("test"), Value: aws.String("tag2")},
								{Key: aws.String("test"), Value: aws.String("tag4")},
								{Key: aws.String("datadoghq.com/dbm"), Value: aws.String("true")},
							},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag", "test:tag2"},
			expectedInstances: []Instance{
				{
					ID:         "test-instance",
					Endpoint:   "test-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
				}, {
					ID:         "test-instance-2",
					Endpoint:   "test-endpoint-2",
					Port:       5432,
					IamEnabled: true,
					Engine:     "aurora-postgresql",
					DbmEnabled: true,
					DbName:     "postgres",
				},
			},
		},
		{
			name: "multiple pages returns instances from all pages",
			configureClient: func(k *MockrdsService) {
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					Marker: aws.String("next"),
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("aurora-postgresql"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
				k.EXPECT().DescribeDBInstances(gomock.Any(), &rds.DescribeDBInstancesInput{
					Marker: aws.String("next"),
					Filters: []types.Filter{
						{
							Name:   aws.String("engine"),
							Values: []string{mysqlEngine, postgresEngine, auroraMysqlEngine, auroraPostgresqlEngine},
						},
					},
				}).Return(&rds.DescribeDBInstancesOutput{
					DBInstances: []types.DBInstance{
						{
							Endpoint: &types.Endpoint{
								Address: aws.String("test-endpoint-2"),
								Port:    aws.Int32(5432),
							},
							DBInstanceIdentifier:             aws.String("test-instance-2"),
							IAMDatabaseAuthenticationEnabled: aws.Bool(true),
							AvailabilityZone:                 aws.String("us-east-1a"),
							DBInstanceStatus:                 aws.String("available"),
							Engine:                           aws.String("postgres"),
							TagList:                          []types.Tag{{Key: aws.String("test"), Value: aws.String("tag")}},
						},
					},
				}, nil).Times(1)
			},
			tags: []string{"test:tag"},
			expectedInstances: []Instance{
				{
					ID:         "test-instance",
					Endpoint:   "test-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "aurora-postgresql",
					DbmEnabled: false,
					DbName:     "postgres",
				}, {
					ID:         "test-instance-2",
					Endpoint:   "test-endpoint-2",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbmEnabled: false,
					DbName:     "postgres",
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
			clusters, err := client.GetRdsInstancesFromTags(context.Background(), tt.tags, defaultDbmTag)
			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
				return
			}
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedInstances, clusters)
		})
	}
}
