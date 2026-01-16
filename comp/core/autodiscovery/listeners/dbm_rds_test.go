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
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	"github.com/go-viper/mapstructure/v2"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBMRdsListener(t *testing.T) {
	testCases := []struct {
		name                  string
		config                map[string]interface{}
		numDiscoveryIntervals int
		rdsClientConfigurer   mockRdsClientConfigurer
		expectedServices      []*DBMRdsService
		expectedDelServices   []*DBMRdsService
	}{
		{
			name: "GetRdsInstancesFromTags context deadline exceeded produces no services",
			config: map[string]interface{}{
				"discoveryInterval": 1,
				"queryTimeout":      1,
				"region":            "us-east-1",
				"tags":              []string{defaultADTag},
				"dbmTag":            defaultDbmTag,
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRdsClient) {
				k.EXPECT().GetRdsInstancesFromTags(
					contextWithTimeout(1*time.Second),
					aws.Config{
						DiscoveryInterval: 1,
						QueryTimeout:      1,
						Region:            "us-east-1",
						Tags:              []string{defaultADTag},
						DbmTag:            defaultDbmTag,
					}).DoAndReturn(
					func(ctx context.Context, _ aws.Config) ([]aws.Instance, error) {
						fmt.Println("called get rds instances from tags")
						<-ctx.Done()
						return nil, ctx.Err()
					}).AnyTimes()
			},
			expectedServices:    []*DBMRdsService{},
			expectedDelServices: []*DBMRdsService{},
		},
		{
			name: "GetRdsInstancesFromTags error produces no services",
			config: map[string]interface{}{
				"discoveryInterval": 1,
				"region":            "us-east-1",
				"tags":              []string{defaultADTag},
				"dbmTag":            defaultDbmTag,
			},
			numDiscoveryIntervals: 0,
			rdsClientConfigurer: func(k *aws.MockRdsClient) {
				k.EXPECT().GetRdsInstancesFromTags(gomock.Any(), aws.Config{
					DiscoveryInterval: 1,
					Region:            "us-east-1",
					Tags:              []string{defaultADTag},
					DbmTag:            defaultDbmTag,
				}).Return(nil, errors.New("big bad error")).AnyTimes()
			},
			expectedServices:    []*DBMRdsService{},
			expectedDelServices: []*DBMRdsService{},
		},
		{
			name: "single endpoint discovered and created",
			config: map[string]interface{}{
				"discoveryInterval": 1,
				"region":            "us-east-1",
				"tags":              []string{defaultADTag},
				"dbmTag":            defaultDbmTag,
			},
			numDiscoveryIntervals: 1,
			rdsClientConfigurer: func(k *aws.MockRdsClient) {
				k.EXPECT().GetRdsInstancesFromTags(gomock.Any(), aws.Config{
					DiscoveryInterval: 1,
					Region:            "us-east-1",
					Tags:              []string{defaultADTag},
					DbmTag:            defaultDbmTag,
				}).Return(
					[]aws.Instance{
						{
							ID:         "my-instance-1",
							Endpoint:   "my-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "postgres",
							DbmEnabled: true,
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMRdsService{
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "36740c31448ee889",
					checkName:    "postgres",
					region:       "us-east-1",
					instance: &aws.Instance{
						ID:         "my-instance-1",
						Endpoint:   "my-endpoint",
						Port:       5432,
						IamEnabled: true,
						Engine:     "postgres",
						DbmEnabled: true,
					},
				},
			},
			expectedDelServices: []*DBMRdsService{},
		},
		{
			name: "multiple instances discovered and created",
			config: map[string]interface{}{
				"discoveryInterval": 1,
				"region":            "us-east-1",
				"tags":              []string{defaultADTag},
				"dbmTag":            defaultDbmTag,
			},
			numDiscoveryIntervals: 1,
			rdsClientConfigurer: func(k *aws.MockRdsClient) {
				k.EXPECT().GetRdsInstancesFromTags(gomock.Any(), aws.Config{
					DiscoveryInterval: 1,
					Region:            "us-east-1",
					Tags:              []string{defaultADTag},
					DbmTag:            defaultDbmTag,
				}).Return(
					[]aws.Instance{
						{
							ID:         "my-instance-1",
							Endpoint:   "my-endpoint",
							Port:       5432,
							IamEnabled: true,
							Engine:     "postgres",
							DbmEnabled: true,
						},
						{
							ID:         "my-instance-2",
							Endpoint:   "my-endpoint-2",
							Port:       5432,
							IamEnabled: true,
							Engine:     "postgres",
							DbmEnabled: true,
						},
					}, nil).AnyTimes()
			},
			expectedServices: []*DBMRdsService{
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "36740c31448ee889",
					checkName:    "postgres",
					region:       "us-east-1",
					instance: &aws.Instance{
						ID:         "my-instance-1",
						Endpoint:   "my-endpoint",
						Port:       5432,
						IamEnabled: true,
						Engine:     "postgres",
						DbmEnabled: true,
					},
				},
				{
					adIdentifier: dbmPostgresADIdentifier,
					entityID:     "1c8d0531614580ed",
					checkName:    "postgres",
					region:       "us-east-1",
					instance: &aws.Instance{
						ID:         "my-instance-2",
						Endpoint:   "my-endpoint-2",
						Port:       5432,
						IamEnabled: true,
						Engine:     "postgres",
						DbmEnabled: true,
					},
				},
			},
			expectedDelServices: []*DBMRdsService{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			newSvc := make(chan Service, 10)
			delSvc := make(chan Service, 10)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("autodiscover_rds_instances", tc.config)
			mockAWSClient := aws.NewMockRdsClient(ctrl)
			tc.rdsClientConfigurer(mockAWSClient)
			ticks := make(chan time.Time, 1)
			var newRdsConfig aws.Config
			err := mapstructure.Decode(tc.config, &newRdsConfig)
			assert.NoError(t, err)
			l := newDBMRdsListener(newRdsConfig, mockAWSClient, ticks)
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
			createdServices := make([]*DBMRdsService, 0)
			for newService := range newSvc {
				dbmRdsService, ok := newService.(*DBMRdsService)
				if !ok {
					require.Fail(t, "received service of incorrect type on service chan")
				}
				createdServices = append(createdServices, dbmRdsService)
			}
			assert.Equal(t, len(tc.expectedServices), len(createdServices))
			assert.ElementsMatch(t, tc.expectedServices, createdServices)

			// assert that the expected deleted services were created
			deletedServices := make([]*DBMRdsService, 0)
			for delService := range delSvc {
				dbmRdsService, ok := delService.(*DBMRdsService)
				if !ok {
					require.Fail(t, "received service of incorrect type on service chan")
				}
				deletedServices = append(deletedServices, dbmRdsService)
			}
			assert.Equal(t, len(tc.expectedDelServices), len(deletedServices))
			assert.ElementsMatch(t, tc.expectedDelServices, deletedServices)
		})
	}
}

func TestGetExtraRdsConfig(t *testing.T) {
	testCases := []struct {
		service       *DBMRdsService
		expectedExtra map[string]string
	}{
		{
			service: &DBMRdsService{
				adIdentifier: dbmPostgresADIdentifier,
				entityID:     "f7fee36c58e3da8a",
				checkName:    "postgres",
				region:       "us-east-1",
				instance: &aws.Instance{
					ID:         "my-instance-1",
					Endpoint:   "my-endpoint",
					Port:       5432,
					IamEnabled: true,
					Engine:     "postgres",
					DbName:     "app",
					DbmEnabled: true,
				},
			},
			expectedExtra: map[string]string{
				"dbname":                         "app",
				"region":                         "us-east-1",
				"managed_authentication_enabled": "true",
				"dbinstanceidentifier":           "my-instance-1",
				"dbm":                            "true",
			},
		},
	}

	for _, tc := range testCases {
		for key, value := range tc.expectedExtra {
			v, err := tc.service.GetExtraConfig(key)
			assert.NoError(t, err)
			assert.Equal(t, value, v)
		}
	}
}
