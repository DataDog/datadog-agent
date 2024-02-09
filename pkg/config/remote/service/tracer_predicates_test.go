// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestTracerPredicateBadTracerVersion(t *testing.T) {
	assert := assert.New(t)

	configs, err := executePredicate(
		&pbgo.Client{
			IsTracer:     true,
			ClientTracer: &pbgo.ClientTracer{TracerVersion: "1.0"},
		},
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: "abc",
			},
		},
	)

	assert.False(configs)
	assert.Error(err)
}

func testPredicate(client *pbgo.Client, assert *assert.Assertions) func(bool, []*pbgo.TracerPredicateV1) {
	return func(result bool, predicates []*pbgo.TracerPredicateV1) {
		config, err := executePredicate(client, predicates)

		if !result {
			assert.False(config)
			return
		}
		assert.NoError(err)
		assert.True(config)
	}
}

func TestTracerPredicates(t *testing.T) {
	assert := assert.New(t)

	client := &pbgo.Client{
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:     "client-id",
			Language:      "python",
			TracerVersion: "1.0",
			Service:       "api",
			Env:           "prod",
			AppVersion:    "12-beta-alpha-ohnoes",
		},
	}
	tester := testPredicate(client, assert)

	tracerVersion := ">=1.0"
	tracerVersionFail := "3.0"
	serviceMatch := "api"
	serviceFail := "api2"
	environment := "prod"
	appVersion := "12-beta-alpha-ohnoes"
	language := "python"
	empty := ""

	tester(true, []*pbgo.TracerPredicateV1{{TracerVersion: tracerVersion}})
	tester(true, []*pbgo.TracerPredicateV1{{Service: serviceMatch}})
	tester(true, []*pbgo.TracerPredicateV1{{Environment: environment}})
	tester(true, []*pbgo.TracerPredicateV1{{AppVersion: appVersion}})
	tester(true, []*pbgo.TracerPredicateV1{{Language: language}})

	tester(false, []*pbgo.TracerPredicateV1{{TracerVersion: tracerVersionFail}})
	tester(false, []*pbgo.TracerPredicateV1{{Service: serviceFail}})
	tester(false, []*pbgo.TracerPredicateV1{{Environment: serviceFail}})
	tester(false, []*pbgo.TracerPredicateV1{{AppVersion: serviceFail}})
	tester(false, []*pbgo.TracerPredicateV1{{Language: serviceFail}})

	// empty string match
	tester(true, []*pbgo.TracerPredicateV1{{Language: empty}})

	// test match everything
	tester(true, []*pbgo.TracerPredicateV1{{}})

	// test match everything
	tester(true, []*pbgo.TracerPredicateV1{})

	// multiple fields
	tester(
		true,
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: tracerVersion,
				Service:       serviceMatch,
			},
		},
	)
	tester(
		false,
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: tracerVersion,
				Service:       serviceFail,
			},
		},
	)
	tester(
		true,
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: tracerVersion,
				Service:       empty,
			},
		},
	)

	// multiple predicates, none matching
	tester(
		false,
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: tracerVersion,
				Service:       serviceFail,
			},
			{
				TracerVersion: tracerVersionFail,
				Service:       serviceMatch,
			},
		},
	)

	// multple predicates, at least one matching
	tester(
		false,
		[]*pbgo.TracerPredicateV1{
			{
				TracerVersion: tracerVersion,
				Service:       serviceFail,
			},
			{
				TracerVersion: tracerVersionFail,
				Service:       serviceMatch,
			},
			{
				TracerVersion: tracerVersionFail,
				Service:       serviceMatch,
			},
		},
	)
}
