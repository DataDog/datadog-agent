package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsertionSort(t *testing.T) {
	assert := assert.New(t)

	tags := []string{
		"zzz",
		"hello:world",
		"world:hello",
		"random2:value",
		"random1:value",
	}

	InsertionSort(tags)

	assert.Equal("hello:world", tags[0])
	assert.Equal("random1:value", tags[1])
	assert.Equal("random2:value", tags[2])
	assert.Equal("world:hello", tags[3])
	assert.Equal("zzz", tags[4])
}
