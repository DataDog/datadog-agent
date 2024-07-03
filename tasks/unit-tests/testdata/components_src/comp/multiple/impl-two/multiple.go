package multipleimpl

import (
    multiple "github.com/DataDog/datadog-agent/comp/multiple/def"
)

type Requires struct {}

type Provides struct {
    Comp multiple.Component
}

type implementation2 struct {}

func NewComponent(reqs Requires) Provides {
    return Provides{
        Comp: &implementation2{}
    }
}