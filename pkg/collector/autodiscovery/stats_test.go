package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

func TestNewAcErrorStats(t *testing.T) {
	s := newAcErrorStats()
	assert.Len(t, s.loader, 0)
	assert.Len(t, s.run, 0)
}

func TestSetLoaderError(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	s.setLoaderError("anotherCheck", "aLoader", "anError")

	assert.Len(t, s.loader, 2) // 2 checks for this loader
	assert.Len(t, s.loader["aCheck"], 1)
	assert.Len(t, s.loader["anotherCheck"], 1)
}

func TestRemoveLoaderErrors(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	s.removeLoaderErrors("aCheck")

	assert.Len(t, s.loader, 0)
}

func TestGetLoaderErrors(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	errs := s.getLoaderErrors()
	assert.Len(t, errs, 1)
}

func TestSetRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	s.setRunError(id, "anotherError")

	assert.Len(t, s.run, 1)
	assert.Equal(t, s.run[id], "anotherError")
}

func TestRemoveRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	s.removeRunError(id)
	assert.Len(t, s.run, 0)
}

func TestGetRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	err := s.getRunErrors()

	assert.Len(t, err, 1)
}
