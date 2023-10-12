// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	"github.com/stretchr/testify/assert"
)

func TestDiagnoseAllBasicRegAndRunNoDiagnoses(t *testing.T) {

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunNoDiagnoses", func(cfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {
		return nil
	})

	diagCfg := diagnosis.Config{
		Include:  []string{"TestDiagnoseAllBasicRegAndRunNoDiagnoses"},
		RunLocal: true,
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	diagnoses, err := Run(diagCfg, senderManager)
	assert.NoError(t, err)
	assert.Len(t, diagnoses, 0)
}

func TestDiagnoseAllBasicRegAndRunSomeDiagnosis(t *testing.T) {

	inDiagnoses := []diagnosis.Diagnosis{
		{
			Result:      diagnosis.DiagnosisSuccess,
			Name:        "Name_foo",
			Category:    "Category_foo",
			Description: "Description_foo",
			Remediation: "Remediation_foo",
			RawError:    "Error_foo",
		},
		{
			Result:      diagnosis.DiagnosisFail,
			Name:        "Name_bar",
			Category:    "Category_bar",
			Description: "Description_bar",
			Remediation: "Remediation_bar",
			RawError:    "Error_bar",
		},
	}

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunSomeDiagnosis-a", func(cfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {
		return inDiagnoses
	})

	diagnosis.Register("TestDiagnoseAllBasicRegAndRunSomeDiagnosis-b", func(cfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {
		return inDiagnoses
	})

	// Include and run
	diagCfgInclude := diagnosis.Config{
		Include:  []string{"TestDiagnoseAllBasicRegAndRunSomeDiagnosis"},
		RunLocal: true,
	}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	outSuitesDiagnosesInclude, err := Run(diagCfgInclude, senderManager)
	assert.NoError(t, err)
	assert.Len(t, outSuitesDiagnosesInclude, 2)
	assert.Equal(t, outSuitesDiagnosesInclude[0].SuiteDiagnoses, inDiagnoses)
	assert.Equal(t, outSuitesDiagnosesInclude[1].SuiteDiagnoses, inDiagnoses)

	// Include and Exclude and run
	diagCfgIncludeExclude := diagnosis.Config{
		Include:  []string{"TestDiagnoseAllBasicRegAndRunSomeDiagnosis"},
		Exclude:  []string{"TestDiagnoseAllBasicRegAndRunSomeDiagnosis-a"},
		RunLocal: true,
	}
	outSuitesDiagnosesIncludeExclude, err := Run(diagCfgIncludeExclude, senderManager)
	assert.NoError(t, err)
	assert.Len(t, outSuitesDiagnosesIncludeExclude, 1)
	assert.Equal(t, outSuitesDiagnosesIncludeExclude[0].SuiteDiagnoses, inDiagnoses)
	assert.Equal(t, outSuitesDiagnosesIncludeExclude[0].SuiteName, "TestDiagnoseAllBasicRegAndRunSomeDiagnosis-b")
}
