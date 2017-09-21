package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectedCollectors_String(t *testing.T) {
	sc := &SelectedCollectors{
		"foo": struct{}{},
		"bar": struct{}{},
	}
	assert.Equal(t, "[foo bar]", sc.String())
}
