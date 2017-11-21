package gui

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadConfDir(t *testing.T) {
	files, err := readConfDir("testdata")
	assert.Nil(t, err)

	sort.Strings(files)
	expected := []string{
		"check.yaml",
		"check.yaml.default",
		"check.yaml.example",
		"foo: conf.yaml",
		"foo: conf.yaml.default",
		"foo: conf.yaml.example",
		"foo: metrics.yaml",
	}

	assert.Equal(t, expected, files)
}
