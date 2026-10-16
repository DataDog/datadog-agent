package smb2

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/auth"
)

// Credentials creates a fresh Initiator for an SMB server. Initiators contain
// handshake state and must not be reused between sessions. Credentials
// implementations must be safe for concurrent use.
type Credentials interface {
	NewInitiator(context.Context, string) (auth.Initiator, error)
}
