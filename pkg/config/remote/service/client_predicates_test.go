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
		[]*clientPredicate{
			{
				TracerVersion: &version,
			},
		},
	)

	assert.False(configs)
	assert.Error(err)
}

func testPredicate(client *pbgo.Client, assert *assert.Assertions) func(bool, []*clientPredicate) {
	return func(result bool, predicates []*clientPredicate) {
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

	tester(true, []*clientPredicate{{TracerVersion: &tracerVersion}})
	tester(true, []*clientPredicate{{Service: &serviceMatch}})
	tester(true, []*clientPredicate{{Environment: &environment}})
	tester(true, []*clientPredicate{{AppVersion: &appVersion}})
	tester(true, []*clientPredicate{{Language: &language}})

	tester(false, []*clientPredicate{{TracerVersion: &tracerVersionFail}})
	tester(false, []*clientPredicate{{Service: &serviceFail}})
	tester(false, []*clientPredicate{{Environment: &serviceFail}})
	tester(false, []*clientPredicate{{AppVersion: &serviceFail}})
	tester(false, []*clientPredicate{{Language: &serviceFail}})

	// empty string match
	tester(false, []*clientPredicate{{Language: &empty}})

	// test match all
	tester(true, []*clientPredicate{})

	// multiple fields
	tester(
		true,
		[]*clientPredicate{
			{
				TracerVersion: &tracerVersion,
				Service:       &serviceMatch,
			},
		},
	)
	tester(
		false,
		[]*clientPredicate{
			{
				TracerVersion: &tracerVersion,
				Service:       &serviceFail,
			},
		},
	)
	tester(
		false,
		[]*clientPredicate{
			{
				TracerVersion: &tracerVersion,
				Service:       &empty,
			},
		},
	)
}
