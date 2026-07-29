// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"errors"
	"net/http/httptest"
	"testing"
)

type statusMock struct {
	sections          []string
	requestedSections []string
	format            string
	verbose           bool
	err               error
}

func (s *statusMock) GetStatus(format string, verbose bool, _ ...string) ([]byte, error) {
	s.format = format
	s.verbose = verbose
	return []byte("all status"), s.err
}

func (s *statusMock) GetSections() []string {
	return s.sections
}

func (s *statusMock) GetStatusBySections(sections []string, format string, verbose bool) ([]byte, error) {
	s.requestedSections = sections
	s.format = format
	s.verbose = verbose
	return []byte("section status"), s.err
}

func TestGetStatusSection(t *testing.T) {
	status := &statusMock{}
	req := httptest.NewRequest("GET", "/status/section/Admission%20Controller?format=text&verbose=true", nil)
	req.SetPathValue("component", "Admission Controller")
	recorder := httptest.NewRecorder()

	getStatusSection(recorder, req, status)

	if recorder.Code != 200 {
		t.Fatalf("expected status code 200, got %d", recorder.Code)
	}
	if recorder.Body.String() != "section status" {
		t.Fatalf("expected section status response, got %q", recorder.Body.String())
	}
	if len(status.requestedSections) != 1 || status.requestedSections[0] != "Admission Controller" {
		t.Fatalf("unexpected requested sections: %v", status.requestedSections)
	}
	if status.format != "text" || !status.verbose {
		t.Fatalf("unexpected status options: format=%q verbose=%v", status.format, status.verbose)
	}
}

func TestGetStatusSections(t *testing.T) {
	status := &statusMock{sections: []string{"header", "Admission Controller"}}
	recorder := httptest.NewRecorder()

	getStatusSections(recorder, httptest.NewRequest("GET", "/status/sections", nil), status)

	if recorder.Code != 200 {
		t.Fatalf("expected status code 200, got %d", recorder.Code)
	}
	if recorder.Body.String() != "[\"header\",\"Admission Controller\"]" {
		t.Fatalf("unexpected sections response: %q", recorder.Body.String())
	}
}

func TestGetStatusSectionError(t *testing.T) {
	status := &statusMock{err: errors.New("unknown status section")}
	req := httptest.NewRequest("GET", "/status/section/unknown?format=json", nil)
	req.SetPathValue("component", "unknown")
	recorder := httptest.NewRecorder()

	getStatusSection(recorder, req, status)

	if recorder.Code != 500 {
		t.Fatalf("expected status code 500, got %d", recorder.Code)
	}
}
