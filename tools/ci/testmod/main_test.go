package testmod

import (
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestAplusB(t *testing.T) {
	assert.Equal(t, 3, aplusb(1, 2))
}

func TestAplusBVar(t *testing.T) {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey != "" {
		t.Fatal("DD_API_KEY is set")
	}
	assert.Equal(t, 42, aplusb(40, 2))
}
