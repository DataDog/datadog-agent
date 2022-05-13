package service

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func TestClientPredicateBadTracerVersion(t *testing.T) {
	assert := assert.New(t)

	configs, err := executePredicate(
		&pbgo.Client{
			IsTracer:     true,
			ClientTracer: &pbgo.ClientTracer{TracerVersion: "1.0"},
		},
		[]*pbgo.TracerPredicate{
			{
				TracerVersion: "abc",
			},
		},
	)

	assert.False(configs)
	assert.Error(err)
}

func testPredicate(client *pbgo.Client, assert *assert.Assertions) func(bool, []*pbgo.TracerPredicate) {
	return func(result bool, predicates []*pbgo.TracerPredicate) {
		config, err := executePredicate(client, predicates)

		if !result {
			assert.False(config)
			return
		}
		assert.NoError(err)
		assert.True(config)
	}
}

func TestClientPredicates(t *testing.T) {
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

	tester(true, []*pbgo.TracerPredicate{{TracerVersion: tracerVersion}})
	tester(true, []*pbgo.TracerPredicate{{Service: serviceMatch}})
	tester(true, []*pbgo.TracerPredicate{{Environment: environment}})
	tester(true, []*pbgo.TracerPredicate{{AppVersion: appVersion}})
	tester(true, []*pbgo.TracerPredicate{{Language: language}})

	tester(false, []*pbgo.TracerPredicate{{TracerVersion: tracerVersionFail}})
	tester(false, []*pbgo.TracerPredicate{{Service: serviceFail}})
	tester(false, []*pbgo.TracerPredicate{{Environment: serviceFail}})
	tester(false, []*pbgo.TracerPredicate{{AppVersion: serviceFail}})
	tester(false, []*pbgo.TracerPredicate{{Language: serviceFail}})

	// empty string match
	tester(true, []*pbgo.TracerPredicate{{Language: empty}})

	// test match all
	tester(true, []*pbgo.TracerPredicate{})

	// multiple fields
	tester(
		true,
		[]*pbgo.TracerPredicate{
			{
				TracerVersion: tracerVersion,
				Service:       serviceMatch,
			},
		},
	)
	tester(
		false,
		[]*pbgo.TracerPredicate{
			{
				TracerVersion: tracerVersion,
				Service:       serviceFail,
			},
		},
	)
	tester(
		true,
		[]*pbgo.TracerPredicate{
			{
				TracerVersion: tracerVersion,
				Service:       empty,
			},
		},
	)
}
