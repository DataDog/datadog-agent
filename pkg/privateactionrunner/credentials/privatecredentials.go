package credentials

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/google/uuid"
)

type PrivateCredentialResolver interface {
	ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactions.ConnectionInfo, userUUID *uuid.UUID) (interface{}, error)
}

type privateCredentialResolver struct {
}

func NewPrivateCredentialResolver() PrivateCredentialResolver {
	return &privateCredentialResolver{}
}

func (p privateCredentialResolver) ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactions.ConnectionInfo, userUUID *uuid.UUID) (interface{}, error) {
	return nil, fmt.Errorf("private connection resolution is not supported yet")
}
