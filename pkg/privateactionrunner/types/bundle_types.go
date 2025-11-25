package types

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type Bundle interface {
	GetAction(actionName string) Action
}

type Action interface {
	Run(
		ctx context.Context,
		task *Task,
		credential *privateconnection.PrivateCredentials,
	) (interface{}, error)
}
