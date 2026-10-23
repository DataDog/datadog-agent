package protocol

import (
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// CrossShareSymlinkError reports a symbolic link whose target points to a UNC
// path outside the current share. Path is the actual UNC used by the CREATE;
// ResolvedPath already includes its suffix.
type CrossShareSymlinkError struct {
	Path         string // Actual path used by the stopped CREATE.
	Target       string // Symbolic-link target, normalized as a user-visible path.
	Relative     bool   // Whether the target was marked relative by the server.
	UnparsedPath string // Suffix the server did not parse at the link.
	ResolvedPath string // Target with UnparsedPath appended.
	err          error
}

func (e *CrossShareSymlinkError) Error() string {
	if e == nil {
		return "empty error"
	}
	return fmt.Sprintf("symbolic link at %q points to %q", e.Path, e.Target)
}
func (e *CrossShareSymlinkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// DFSReferralRequiredError reports a DFS CREATE stopped with STATUS_PATH_NOT_COVERED,
// indicating that resolving the path requires a DFS referral.
type DFSReferralRequiredError struct {
	Path         string // Full UNC path used by the stopped CREATE.
	ReparsePoint bool   // Whether the stopped CREATE requested FILE_OPEN_REPARSE_POINT.
	err          error
}

func (e *DFSReferralRequiredError) Error() string {
	if e == nil {
		return "empty error"
	}
	return fmt.Sprintf("DFS referral required for %q", e.Path)
}
func (e *DFSReferralRequiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// TransportError represents a error come from net.Conn layer.
type TransportError struct {
	Err error
}

func (err *TransportError) Error() string {
	if err == nil {
		return "empty error"
	}
	return fmt.Sprintf("connection error: %v", err.Err)
}

func (err *TransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// InvalidResponseError reports malformed or unexpected data from the server.
type InvalidResponseError struct {
	// Command is the expected request command, or nil when it is unknown.
	Command *wire.Command
	Message string
}

func (err *InvalidResponseError) Error() string {
	if err == nil {
		return "empty error"
	}
	if err.Command == nil {
		return fmt.Sprintf("protocol: invalid response: %s", err.Message)
	}
	return fmt.Sprintf("protocol: invalid %s response: %s", *err.Command, err.Message)
}

func invalidResponse(command wire.Command, message string) *InvalidResponseError {
	return &InvalidResponseError{Command: &command, Message: message}
}

// Attach request context without modifying an error shared by multiple requests.
func withResponseCommand(err error, command wire.Command) error {
	if invalid, ok := err.(*InvalidResponseError); ok && invalid.Command == nil {
		return invalidResponse(command, invalid.Message)
	}
	return err
}

// ResponseError represents a error with a nt status code sent by the server.
// The NTSTATUS is defined in [MS-ERREF].
// https://msdn.microsoft.com/en-au/library/cc704588.aspx
type ResponseError struct {
	Code uint32 // NTSTATUS
	data [][]byte

	// requiredBufferLength is populated only after conn.accept validates the
	// QUERY_INFO error-data format. It is intentionally private so callers
	// cannot mistake arbitrary server error data for a retry instruction.
	requiredBufferLength uint32
}

// requireBufferLength returns the required buffer length reported by the server,
// if any, for errors such as STATUS_BUFFER_TOO_SMALL or STATUS_INFO_LENGTH_MISMATCH.
func (err *ResponseError) requireBufferLength() (int, bool) {
	if err == nil || err.requiredBufferLength == 0 {
		return 0, false
	}
	return int(err.requiredBufferLength), true
}

func (err ResponseError) Error() string {
	return fmt.Sprintf("response error: %v", erref.NtStatus(err.Code))
}

func (err ResponseError) Unwrap() error {
	switch erref.NtStatus(err.Code) {
	case erref.STATUS_OBJECT_NAME_NOT_FOUND,
		erref.STATUS_OBJECT_PATH_NOT_FOUND:
		return os.ErrNotExist
	case erref.STATUS_OBJECT_NAME_COLLISION:
		return os.ErrExist
	case erref.STATUS_ACCESS_DENIED,
		erref.STATUS_CANNOT_DELETE,
		erref.STATUS_NETWORK_ACCESS_DENIED:
		return os.ErrPermission
	case erref.STATUS_FILE_CLOSED:
		return os.ErrClosed
	}
	return nil
}

func (err ResponseError) As(target any) bool {
	if p, ok := target.(*erref.NtStatus); ok {
		*p = erref.NtStatus(err.Code)
		return true
	}
	return false
}

func (err ResponseError) Is(target error) bool {
	if status, ok := target.(erref.NtStatus); ok {
		return erref.NtStatus(err.Code) == status
	}
	if unwrapped := err.Unwrap(); unwrapped != nil {
		return errors.Is(unwrapped, target)
	}
	return false
}

// CompoundResponseError represents errors that occurred during execution of a compound request.
type CompoundResponseError struct {
	Errors []error
}

func (e *CompoundResponseError) Error() string {
	if e == nil {
		return "empty error"
	}
	var b []byte
	for _, err := range e.Errors {
		b = append(b, '\n')
		if err != nil {
			b = append(b, err.Error()...)
		}
	}
	if len(b) == 0 {
		return "empty error"
	}
	return string(b)
}

func (e *CompoundResponseError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Errors
}

func (e *CompoundResponseError) OpError(i int) error {
	if e == nil || i < 0 || i >= len(e.Errors) {
		return nil
	}
	return e.Errors[i]
}

// requireBufferLength returns the required buffer length from the failed operation at index i in the compound.
func (e *CompoundResponseError) requireBufferLength(i int) (int, bool) {
	if e == nil {
		return 0, false
	}
	// A compound's later operations only reference the handle opened by op 0,
	// so a required buffer length from a later op is meaningful only when that
	// opening operation succeeded.
	if i > 0 && len(e.Errors) > 0 && e.Errors[0] != nil {
		return 0, false
	}
	if rerr, ok := errors.AsType[*ResponseError](e.OpError(i)); ok {
		return rerr.requireBufferLength()
	}
	return 0, false
}

// requireBufferLength returns the required buffer length reported by the server
// for operation i, if err indicates that a query buffer was too small (such as
// STATUS_BUFFER_TOO_SMALL or STATUS_INFO_LENGTH_MISMATCH).
func requireBufferLength(err error, i int) (int, bool) {
	if cerr, ok := errors.AsType[*CompoundResponseError](err); ok {
		return cerr.requireBufferLength(i)
	}
	if i != 0 {
		return 0, false
	}
	if rerr, ok := errors.AsType[*ResponseError](err); ok {
		return rerr.requireBufferLength()
	}
	return 0, false
}

// RequiredBufferLength reports the server's required output size for the
// failed operation at index i.
func RequiredBufferLength(err error, i int) (int, bool) {
	return requireBufferLength(err, i)
}

// bufferOverflowData returns the partial output carried by a
// STATUS_BUFFER_OVERFLOW Response. Servers return the truncated result in the
// response body so callers can use it without requesting a larger buffer.
func bufferOverflowData(err error) ([]byte, bool) {
	rerr, ok := errors.AsType[*ResponseError](err)
	if !ok || rerr == nil || erref.NtStatus(rerr.Code) != erref.STATUS_BUFFER_OVERFLOW || len(rerr.data) == 0 {
		return nil, false
	}
	return rerr.data[0], true
}

// BufferOverflowData returns partial output carried by STATUS_BUFFER_OVERFLOW.
func BufferOverflowData(err error) ([]byte, bool) {
	return bufferOverflowData(err)
}

// ResponseErrorAt returns the response error at index in a compound error.
func ResponseErrorAt(err error, index int) *ResponseError {
	return responseErrorAt(err, index)
}
