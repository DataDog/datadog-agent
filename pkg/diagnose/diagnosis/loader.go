// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnosis

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// --------------------------------
// Diagnose (Metadata availability subcommand)
type MetadataAvailDiagnose func() error
type MetadataAvailDiagnoseCatalog map[string]MetadataAvailDiagnose

var MetadataAvailCatalog = make(MetadataAvailDiagnoseCatalog)

func RegisterMetadataAvail(name string, d MetadataAvailDiagnose) {
	if _, ok := MetadataAvailCatalog[name]; ok {
		log.Warnf("Diagnosis %s already registered, overriding it", name)
	}
	MetadataAvailCatalog[name] = d
}

// --------------------------------
// Diagnose (all subcommand)

// Diagnose interface function
type Diagnose func(DiagnoseConfig) []Diagnosis

// Global list of registered Diagnose functions
var DiagnoseCatalog = make([]DiagnoseSuite, 0)

// Diagnose suite information
type DiagnoseSuite struct {
	SuitName string
	Diagnose Diagnose
}

// Diagnose configuration
type DiagnoseConfig struct {
	Verbose        bool
	ForceLocal     bool
	RemoteDiagnose bool
	Include        []*regexp.Regexp
	Exclude        []*regexp.Regexp
}

type DiagnosisResult int

// Use explicit constant instead of iota because the same numbers are used
// in Python/CGO calls.
// Change here needs to be reflected in
//    datadog-agent\rtloader\include\rtloader_types.h
//    integrations-core\datadog_checks_base\datadog_checks\base\utils\diagnose.py

const (
	DiagnosisSuccess         DiagnosisResult = 0
	DiagnosisNotEnable       DiagnosisResult = 1
	DiagnosisFail            DiagnosisResult = 2
	DiagnosisWarning         DiagnosisResult = 3
	DiagnosisUnexpectedError DiagnosisResult = 4
	DiagnosisResultMIN                       = DiagnosisSuccess
	DiagnosisResultMAX                       = DiagnosisUnexpectedError
)

// Diagnose result (diagnosis)
type Diagnosis struct {
	// --------------------------
	// required fields

	// run-time (pass, fail etc)
	Result DiagnosisResult
	// static-time (meta typically)
	Name string
	// run-time (actual diagnosis consumable by a user)
	Diagnosis string

	// --------------------------
	// optional fields

	// static-time (meta typically)
	Category string
	// static-time (meta typically, description of what being tested)
	Description string
	// run-time (what can be done of what docs need to be consulted to address the issue)
	Remediation string
	// run-time
	RawError error
}

type Diagnoses struct {
	SuiteName      string
	SuiteDiagnoses []Diagnosis
}

// Add Diagnose suite
func Register(suiteName string, diagnose Diagnose) {
	DiagnoseCatalog = append(DiagnoseCatalog, DiagnoseSuite{
		SuitName: suiteName,
		Diagnose: diagnose,
	})
}
