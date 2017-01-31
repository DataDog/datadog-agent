package check

import (
	"hash"
	"hash/fnv"
)

// ID is the representation of the unique ID of a Check instance
type ID uint64

// Identifier allows generating the unique ID of a Check
type Identifier struct {
	hash.Hash64
}

// NewIdentifier instantiates a new Identifier and returns a pointer to it
func NewIdentifier() *Identifier {
	return &Identifier{fnv.New64()}
}

// Identify returns the ID of the check
func (ci Identifier) Identify(check Check, instance ConfigData, initConfig ConfigData) ID {
	ci.Reset()
	ci.Write([]byte(check.String()))
	ci.Write([]byte(instance))
	ci.Write([]byte(initConfig))
	return ID(ci.Sum64())
}
