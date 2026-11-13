package protocol

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// Header returns a validated packet header. The returned view is read-only
// and remains valid until Close. No additional IsInvalid check is needed.
func (r *Response) Header(i int) (wire.PacketCodec, error) {
	packet := r.packet(i)
	if packet == nil {
		return nil, errors.New("protocol: response index unavailable")
	}
	header := wire.PacketCodec(packet.bytes())
	if header.IsInvalidResponse() {
		return nil, &InvalidResponseError{Message: "broken packet header"}
	}
	return header, nil
}

type responseDecoder interface {
	~[]byte
	IsInvalid() bool
}

// decodeResponse validates the envelope, not the contents of its raw payload.
func decodeResponse[D responseDecoder](r *Response, i int, command wire.Command) (D, error) {
	header, err := r.Header(i)
	if err != nil {
		return nil, err
	}
	if header.Command() != command {
		return nil, fmt.Errorf("protocol: response at index %d is %s, want %s", i, header.Command(), command)
	}
	if command == wire.SMB2_READ && r.ext(i) != nil {
		return nil, errors.New("protocol: direct READ payload is available through DirectData")
	}
	decoded := D(header.Body())
	if decoded.IsInvalid() {
		return nil, invalidResponse(command, "broken packet body")
	}
	return decoded, nil
}

// Create returns a validated CREATE response decoder. No additional IsInvalid
// check is needed. The returned view is read-only and valid until Close.
func (r *Response) Create(i int) (wire.CreateResponseDecoder, error) {
	return decodeResponse[wire.CreateResponseDecoder](r, i, wire.SMB2_CREATE)
}

// Read returns a validated READ response decoder. No additional IsInvalid
// check is needed. The returned view is read-only and valid until Close.
// For direct reception, use DirectData instead; Read returns an error.
func (r *Response) Read(i int) (wire.ReadResponseDecoder, error) {
	return decodeResponse[wire.ReadResponseDecoder](r, i, wire.SMB2_READ)
}

// Write returns a validated WRITE response decoder. No additional IsInvalid
// check is needed. The returned view is read-only and valid until Close.
func (r *Response) Write(i int) (wire.WriteResponseDecoder, error) {
	return decodeResponse[wire.WriteResponseDecoder](r, i, wire.SMB2_WRITE)
}

// QueryInfo returns a QUERY_INFO envelope with request-aware payload accessors.
// Its views are read-only and valid until the owning Response is closed.
func (r *Response) QueryInfo(i int) (*QueryInfoResponse, error) {
	decoded, err := decodeResponse[wire.QueryInfoResponseDecoder](r, i, wire.SMB2_QUERY_INFO)
	if err != nil {
		return nil, err
	}
	return &QueryInfoResponse{decoded: decoded, request: r.packet(i).payloadRequest}, nil
}

// Ioctl returns an IOCTL envelope with request-aware payload accessors.
// Its views are read-only and valid until the owning Response is closed.
func (r *Response) Ioctl(i int) (*IoctlResponse, error) {
	decoded, err := decodeResponse[wire.IoctlResponseDecoder](r, i, wire.SMB2_IOCTL)
	if err != nil {
		return nil, err
	}
	request := r.packet(i).payloadRequest
	if request.command == wire.SMB2_IOCTL && decoded.CtlCode() != request.ctlCode {
		return nil, invalidResponse(wire.SMB2_IOCTL, "IOCTL response control code does not match request")
	}
	return &IoctlResponse{decoded: decoded, request: request}, nil
}

// QueryDir returns a QUERY_DIRECTORY envelope with request-aware payload accessors.
// Its views are read-only and valid until the owning Response is closed.
func (r *Response) QueryDir(i int) (*QueryDirectoryResponse, error) {
	decoded, err := decodeResponse[wire.QueryDirectoryResponseDecoder](r, i, wire.SMB2_QUERY_DIRECTORY)
	if err != nil {
		return nil, err
	}
	return &QueryDirectoryResponse{decoded: decoded, request: r.packet(i).payloadRequest}, nil
}

// ChangeNotify returns a CHANGE_NOTIFY envelope with request-aware payload accessors.
func (r *Response) ChangeNotify(i int) (*ChangeNotifyResponse, error) {
	decoded, err := decodeResponse[wire.ChangeNotifyResponseDecoder](r, i, wire.SMB2_CHANGE_NOTIFY)
	if err != nil {
		return nil, err
	}
	return &ChangeNotifyResponse{decoded: decoded, request: r.packet(i).payloadRequest}, nil
}
