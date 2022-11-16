// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package rego

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/process"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type regoInputFixture struct {
	name          string
	inputs        []compliance.RegoInput
	processes     processutils.Processes
	expectedInput string
}

const ruleID string = "rule-id"

func (f *regoInputFixture) newRegoCheck() (*regoCheck, error) {
	rule := &compliance.RegoRule{
		RuleCommon: compliance.RuleCommon{
			ID: ruleID,
		},
	}

	inputs := make([]regoInput, len(f.inputs))
	for i, input := range f.inputs {
		inputs[i] = regoInput{RegoInput: input}
	}

	regoCheck := &regoCheck{
		ruleID: ruleID,
		inputs: inputs,
	}

	if err := regoCheck.CompileRule(rule, "", &compliance.SuiteMeta{}, nil); err != nil {
		return nil, err
	}

	return regoCheck, nil
}

func (f *regoInputFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	cache.Cache.Delete("compliance-processes")
	processutils.Fetcher = func() (processutils.Processes, error) {
		return f.processes, nil
	}

	tf, err := os.CreateTemp("", "rego-input-dump")
	assert.NoError(err)
	err = tf.Close()
	assert.NoError(err)
	defer os.Remove(tf.Name())

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("ProvidedInput", mock.Anything).Return(nil).Once()
	env.On("Hostname").Return("hostname_test").Once()
	env.On("DumpInputPath").Return(tf.Name()).Once()
	env.On("ShouldSkipRegoEval").Return(false).Once()

	defer env.AssertExpectations(t)

	regoCheck, err := f.newRegoCheck()
	assert.NoError(err)
	reports := regoCheck.Check(env)
	t.Logf("reports: %+v", reports)

	content, err := os.ReadFile(tf.Name())
	assert.NoError(err)

	t.Logf("content: %v", string(content))

	var res interface{}
	err = json.Unmarshal(content, &res)
	assert.NoError(err)

	var expected interface{}
	err = json.Unmarshal([]byte(f.expectedInput), &expected)
	assert.NoError(err)
	expectedGlobal := map[string]interface{}{
		ruleID: expected,
	}

	assert.Equal(expectedGlobal, res)
}

func TestRegoInputCheck(t *testing.T) {
	tests := []regoInputFixture{
		{
			name: "simple case",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
			},
			expectedInput: `
				{
					"context": {
						"hostname": "hostname_test",
						"ruleID": "rule-id",
						"input": {
							"processes": {
								"process": {
									"name": "proc1"
								},
								"tag": "processes",
								"type": "array"
							}
						}
					},
					"processes": [
						{
							"cmdLine": [
								"arg1",
								"--path=foo"
							],
							"exe": "",
							"flags": {
								"--path": "foo",
								"arg1": ""
							},
							"name": "proc1",
							"pid": 42
						}
					]
				}
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
