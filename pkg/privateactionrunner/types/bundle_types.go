package types

import (
	"context"
)

type Bundle interface {
	GetAction(actionName string) Action
}

type Action interface {
	Run(
		ctx context.Context,
		task *Task,
		credential interface{},
	) (interface{}, error)
}
