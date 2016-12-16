package dogstream

import (
	"os"
	"testing"

	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := Initialize("tests")

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestLoad(t *testing.T) {
	p, err := Load("foo")
	assert.Nil(t, err)

	assert.Nil(t, p.Parse("foo", "bar"))
}

func BenchmarkLoad(b *testing.B) {
	for n := 0; n < b.N; n++ {
		Load("foo")
	}
}

func BenchmarkParse(b *testing.B) {
	p, _ := Load("foo")
	for n := 0; n < b.N; n++ {
		p.Parse("foo", "bar")
	}
}
