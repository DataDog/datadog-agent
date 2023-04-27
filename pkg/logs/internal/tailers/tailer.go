package tailers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
)

// Tailer the base interface for a tailer.
type Tailer interface {
	GetId() string
	GetType() string
	GetInfo() *status.InfoRegistry
}
