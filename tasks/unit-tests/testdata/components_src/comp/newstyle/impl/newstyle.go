package newstyleimpl

import (
    newstyle "github.com/DataDog/datadog-agent/comp/newstyle/def"
)

type Requires struct {}

type Provides struct {
    Comp newstyle.Component
}

type implementation struct {}

func NewComponent(reqs Requires) Provides {
    return Provides{
        Comp: &implementation{}
    }
}