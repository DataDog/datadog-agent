package protocol

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// acceptRequest applies the command-specific response validation which needs
// information from the original request (currently the requested I/O length).
func acceptRequest(rr *outstandingRequest, rp *recvPacket, dialect uint16) (*recvPacket, error) {
	if rr == nil {
		if rp != nil {
			rp.close()
		}
		return nil, errors.New("protocol: nil outstanding request")
	}
	if err := validateRequestedOutput(rr, rp); err != nil {
		rp.close()
		return nil, err
	}
	accepted, err := acceptWithLimits(rr.cmd, rp, dialect, rr.expectedRead, rr.hasExpectedRead, rr.expectedWrite, rr.hasExpectedWrite)
	if err == nil {
		accepted.payloadRequest = rr.payloadRequest
	}
	return accepted, err
}

func validateRequestedOutput(rr *outstandingRequest, rp *recvPacket) error {
	if rr == nil || rp == nil {
		return nil
	}
	status := erref.NtStatus(rp.codec().Status())
	switch rr.cmd {
	case wire.SMB2_QUERY_INFO:
		if status != erref.STATUS_SUCCESS && status != erref.STATUS_BUFFER_OVERFLOW {
			return nil
		}
		r := wire.QueryInfoResponseDecoder(rp.codec().Body())
		if !r.IsInvalid() && r.OutputBufferLength() > rr.payloadRequest.maxOutput {
			return invalidResponse(rr.cmd, "query info output exceeds requested length")
		}
	case wire.SMB2_QUERY_DIRECTORY:
		if status != erref.STATUS_SUCCESS {
			return nil
		}
		r := wire.QueryDirectoryResponseDecoder(rp.codec().Body())
		if !r.IsInvalid() && r.OutputBufferLength() > rr.payloadRequest.maxOutput {
			return invalidResponse(rr.cmd, "query directory output exceeds requested length")
		}
	case wire.SMB2_IOCTL:
		if status != erref.STATUS_SUCCESS && status != erref.STATUS_BUFFER_OVERFLOW {
			return nil
		}
		r := wire.IoctlResponseDecoder(rp.codec().Body())
		if r.IsInvalid() {
			return nil // acceptError handles an SMB2 ErrorResponse fallback.
		}
		if r.OutputCount() > rr.payloadRequest.maxOutput {
			return invalidResponse(wire.SMB2_IOCTL, "IOCTL output exceeds requested length")
		}
		if r.InputCount() > rr.payloadRequest.maxInput {
			return invalidResponse(wire.SMB2_IOCTL, "IOCTL input exceeds requested length")
		}
	case wire.SMB2_CHANGE_NOTIFY:
		if status != erref.STATUS_SUCCESS && status != erref.STATUS_NOTIFY_ENUM_DIR {
			return nil
		}
		r := wire.ChangeNotifyResponseDecoder(rp.codec().Body())
		if r.IsInvalid() {
			return nil
		}
		if r.OutputBufferLength() > rr.payloadRequest.notifyOutput {
			return invalidResponse(wire.SMB2_CHANGE_NOTIFY, "change notify output exceeds requested length")
		}
		if status == erref.STATUS_NOTIFY_ENUM_DIR && r.OutputBufferLength() != 0 {
			return invalidResponse(wire.SMB2_CHANGE_NOTIFY, "broken change notify response format")
		}
	}
	return nil
}

func validateResponseBody(cmd wire.Command, body []byte, dialect uint16, expectedRead uint32, hasRead bool, expectedWrite uint32, hasWrite bool) error {
	invalid := func() error {
		name := cmd.String()
		switch cmd {
		case wire.SMB2_NEGOTIATE:
			name = "negotiate"
		case wire.SMB2_SESSION_SETUP:
			name = "session setup"
		}
		return invalidResponse(cmd, fmt.Sprintf("broken %s response format", name))
	}
	switch cmd {
	case wire.SMB2_NEGOTIATE:
		if wire.NegotiateResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_CREATE:
		if wire.CreateResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_CLOSE:
		if wire.CloseResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_FLUSH:
		if wire.FlushResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_READ:
		r := wire.ReadResponseDecoder(body)
		if r.IsInvalid() {
			return invalid()
		}
		if hasInvalidReadFlags(r, dialect) {
			return invalid()
		}
		if hasRead && r.DataLength() > expectedRead {
			return invalidResponse(wire.SMB2_READ, "read length exceeds requested length")
		}
	case wire.SMB2_WRITE:
		r := wire.WriteResponseDecoder(body)
		if r.IsInvalid() {
			return invalid()
		}
		if hasWrite && r.Count() > expectedWrite {
			return invalidResponse(wire.SMB2_WRITE, "write count exceeds requested length")
		}
	case wire.SMB2_LOCK:
		if wire.LockResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_IOCTL:
		if wire.IoctlResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_QUERY_DIRECTORY:
		if wire.QueryDirectoryResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_CHANGE_NOTIFY:
		if wire.ChangeNotifyResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_QUERY_INFO:
		if wire.QueryInfoResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_SET_INFO:
		if wire.SetInfoResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_TREE_CONNECT:
		if wire.TreeConnectResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_TREE_DISCONNECT:
		if wire.TreeDisconnectResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_ECHO:
		if wire.EchoResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_LOGOFF:
		if wire.LogoffResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	case wire.SMB2_SESSION_SETUP:
		if wire.SessionSetupResponseDecoder(body).IsInvalid() {
			return invalid()
		}
	}
	return nil
}

func validateResponsePacket(cmd wire.Command, rp *recvPacket, dialect uint16, expectedRead uint32, hasRead bool, expectedWrite uint32, hasWrite bool) error {
	if cmd == wire.SMB2_READ && rp != nil && len(rp.ext) != 0 {
		r := wire.ReadResponseDecoder(rp.codec().Body())
		if r.IsInvalidHeader() || len(rp.ext) != int(r.DataLength()) || hasInvalidReadFlags(r, dialect) {
			return invalidResponse(wire.SMB2_READ, "broken read response format")
		}
		if hasRead && r.DataLength() > expectedRead {
			return invalidResponse(wire.SMB2_READ, "read length exceeds requested length")
		}
		return nil
	}
	return validateResponseBody(cmd, rp.codec().Body(), dialect, expectedRead, hasRead, expectedWrite, hasWrite)
}
