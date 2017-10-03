package embed

import (
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// retryExitError converts `exec.ExitError`s to `check.RetryableError`s, so that checks using this
// are retried.
// embed checks must use this from their `Run` method when exit errors need to be retried.
func retryExitError(err error) error {
	switch err.(type) {
	case *exec.ExitError:
		return check.RetryableError{err}
	default:
		return err
	}
}
