package service

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/stretchr/testify/assert"
)

func TestClientPredicateBadTracerVersion(t *testing.T) {
	assert := assert.New(t)

	version := "abc"
	configs, err := executePredicate(
		&pbgo.Client{
			IsTracer:     true,
			ClientTracer: &pbgo.ClientTracer{TracerVersion: "1.0"},
		},
		[]*tracerPredicates{
			{
				TracerVersion: &version,
			},
		},
	)

	assert.False(configs)
	assert.Error(err)
}

func testPredicate(client *pbgo.Client, assert *assert.Assertions) func(bool, []*tracerPredicates) {
	return func(result bool, predicates []*tracerPredicates) {
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

	tester(true, []*tracerPredicates{{TracerVersion: &tracerVersion}})
	tester(true, []*tracerPredicates{{Service: &serviceMatch}})
	tester(true, []*tracerPredicates{{Environment: &environment}})
	tester(true, []*tracerPredicates{{AppVersion: &appVersion}})
	tester(true, []*tracerPredicates{{Language: &language}})

	tester(false, []*tracerPredicates{{TracerVersion: &tracerVersionFail}})
	tester(false, []*tracerPredicates{{Service: &serviceFail}})
	tester(false, []*tracerPredicates{{Environment: &serviceFail}})
	tester(false, []*tracerPredicates{{AppVersion: &serviceFail}})
	tester(false, []*tracerPredicates{{Language: &serviceFail}})

	// empty string match
	tester(false, []*tracerPredicates{{Language: &empty}})

	// test match all
	tester(true, []*tracerPredicates{})

	// multiple fields
	tester(
		true,
		[]*tracerPredicates{
			{
				TracerVersion: &tracerVersion,
				Service:       &serviceMatch,
			},
		},
	)
	tester(
		false,
		[]*tracerPredicates{
			{
				TracerVersion: &tracerVersion,
				Service:       &serviceFail,
			},
		},
	)
	tester(
		false,
		[]*tracerPredicates{
			{
				TracerVersion: &tracerVersion,
				Service:       &empty,
			},
		},
	)
}
