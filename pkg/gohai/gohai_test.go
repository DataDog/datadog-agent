package main

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestSelectedCollectors_String(t *testing.T) {
	sc := &SelectedCollectors{
		"foo" : struct{}{},
		"bar" : struct{}{},
	}
	assert.Equal(t, "[foo bar]", sc.String())
}