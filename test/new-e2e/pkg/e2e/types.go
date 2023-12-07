package e2e

import (
	"testing"
)

type Context interface {
	T() *testing.T
}

type RawResources map[string][]byte

func (rr RawResources) Merge(in RawResources) {
	for k, v := range in {
		rr[k] = v
	}
}

type Initializable interface {
	Init(Context) error
}
