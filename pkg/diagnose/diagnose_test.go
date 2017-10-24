package diagnose

import (
	"bytes"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

type dummySucceedingDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dummySucceedingDiagnosis) Diagnose() error {
	return nil
}

type dummyFailingDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dummyFailingDiagnosis) Diagnose() error {
	return errors.New("fail")
}

func TestDiagnose(t *testing.T) {

	diagnosis.Register("failing", new(dummyFailingDiagnosis))
	diagnosis.Register("succeeding", new(dummySucceedingDiagnosis))

	w := &bytes.Buffer{}
	Diagnose(w)

	expected := `  Diagnosis |
    failing | FAIL |
 succeeding | PASS |
`
	if result := w.String(); result != expected {
		t.Errorf("Got: %v Expected: %v", result, expected)
	}
}
