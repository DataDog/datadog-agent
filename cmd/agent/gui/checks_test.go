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
		"foo.d/conf.yaml",
		"foo.d/conf.yaml.default",
		"foo.d/conf.yaml.example",
		"foo.d/metrics.yaml",
	}

	assert.Equal(t, expected, files)
}

func TestConfigsInPath(t *testing.T) {
	files, err := getConfigsInPath("testdata")
	assert.Nil(t, err)

	sort.Strings(files)
	expected := []string{
		"check.yaml",
		"check.yaml.example",
		"foo.d/conf.yaml",
		"foo.d/conf.yaml.example",
	}

	assert.Equal(t, expected, files)
}
