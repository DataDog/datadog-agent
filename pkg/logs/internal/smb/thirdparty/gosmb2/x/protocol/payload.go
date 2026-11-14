package protocol

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/security"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// payloadRequest retains only the selectors needed to interpret an output.
// It is copied at send time, so reusing a Request cannot change an old response.
type payloadRequest struct {
	command        wire.Command
	infoType       uint8
	infoClass      uint8
	additionalInfo uint32
	ctlCode        uint32
	maxInput       uint32
	maxOutput      uint32
	notifyFlags    uint16
	notifyOutput   uint32
	copyTotal      uint64
	hasCopyTotal   bool
}

func describePayloadRequest(packet wire.Packet) payloadRequest {
	switch req := packet.(type) {
	case *wire.QueryInfoRequest:
		return payloadRequest{command: req.Command(), infoType: req.InfoType, infoClass: req.FileInfoClass, additionalInfo: req.AdditionalInformation, maxOutput: req.OutputBufferLength}
	case *wire.QueryDirectoryRequest:
		return payloadRequest{command: req.Command(), infoClass: req.FileInfoClass, maxOutput: req.OutputBufferLength}
	case *wire.IoctlRequest:
		var copyTotal uint64
		hasCopyTotal := false
		if copy, ok := req.Input.(*wire.SrvCopychunkCopy); ok && copy != nil {
			hasCopyTotal = true
			for _, chunk := range copy.Chunks {
				copyTotal += uint64(chunk.Length)
			}
		}
		return payloadRequest{command: req.Command(), ctlCode: req.CtlCode, maxInput: req.MaxInputResponse, maxOutput: req.MaxOutputResponse, copyTotal: copyTotal, hasCopyTotal: hasCopyTotal}
	case *wire.ChangeNotifyRequest:
		return payloadRequest{command: req.Command(), notifyFlags: req.Flags, notifyOutput: req.OutputBufferLength}
	}
	return payloadRequest{}
}

func decodePayload[D responseDecoder](output []byte, command wire.Command, description string) (D, error) {
	decoded := D(output)
	if decoded.IsInvalid() {
		return nil, invalidResponse(command, "broken "+description)
	}
	return decoded, nil
}

// QueryInfoResponse interprets QUERY_INFO output using the original request.
// Typed accessors validate on access and return read-only views valid until
// Response.Close. Output does not validate the nested payload.
type QueryInfoResponse struct {
	decoded wire.QueryInfoResponseDecoder
	request payloadRequest
}

// Output returns the payload from the validated response envelope without
// validating its contents. The read-only view is valid until Response.Close.
func (r *QueryInfoResponse) Output() []byte {
	if r == nil || r.decoded == nil {
		return nil
	}
	return r.decoded.Output()
}

func (r *QueryInfoResponse) requireClass(infoType, infoClass uint8) error {
	if r == nil || r.decoded == nil || r.request.command != wire.SMB2_QUERY_INFO || r.request.infoType != infoType || r.request.infoClass != infoClass {
		return errors.New("protocol: payload accessor does not match QUERY_INFO request")
	}
	return nil
}

// SecurityDescriptor validates and decodes the security fields requested by
// QUERY_INFO. The returned descriptor owns its data independently of Response.
func (r *QueryInfoResponse) SecurityDescriptor() (*security.Descriptor, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_SECURITY, 0); err != nil {
		return nil, err
	}
	descriptor, err := security.DecodeDescriptor(r.Output(), security.Information(r.request.additionalInfo))
	if err != nil {
		return nil, invalidResponse(wire.SMB2_QUERY_INFO, fmt.Sprintf("broken security descriptor: %v", err))
	}
	return descriptor, nil
}

// QueryDirectoryResponse interprets directory entries using the requested class.
// Typed accessors validate the entire chain before returning any entries.
type QueryDirectoryResponse struct {
	decoded wire.QueryDirectoryResponseDecoder
	request payloadRequest
}

// Output returns the payload from the validated response envelope without
// validating its contents. The read-only view is valid until Response.Close.
func (r *QueryDirectoryResponse) Output() []byte {
	if r == nil || r.decoded == nil {
		return nil
	}
	return r.decoded.Output()
}

// FileIdBothDirectoryInformation returns validated, read-only directory entries.
// The entries remain valid until the owning Response is closed.
func (r *QueryDirectoryResponse) FileIdBothDirectoryInformation() ([]wire.FileIdBothDirectoryInformationDecoder, error) {
	if r == nil || r.decoded == nil || r.request.command != wire.SMB2_QUERY_DIRECTORY || r.request.infoClass != wire.FileIdBothDirectoryInformation {
		return nil, errors.New("protocol: payload accessor does not match QUERY_DIRECTORY request")
	}
	output := r.Output()
	entries := make([]wire.FileIdBothDirectoryInformationDecoder, 0, len(output)/128)
	for len(output) != 0 {
		entry, err := decodePayload[wire.FileIdBothDirectoryInformationDecoder](output, wire.SMB2_QUERY_DIRECTORY, "query directory response format")
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		next := entry.NextEntryOffset()
		if next == 0 {
			break
		}
		output = output[next:]
	}
	return entries, nil
}

// IoctlResponse interprets output using the requested control code. Typed
// accessors validate on access and return read-only views valid until Close.
type IoctlResponse struct {
	decoded wire.IoctlResponseDecoder
	request payloadRequest
}

// ChangeNotifyResponse interprets CHANGE_NOTIFY output using the original
// request. Its typed accessor validates the complete notification chain.
type ChangeNotifyResponse struct {
	decoded wire.ChangeNotifyResponseDecoder
	request payloadRequest
}

// Output returns the payload from the validated response envelope without
// validating its contents. The read-only view is valid until Response.Close.
func (r *ChangeNotifyResponse) Output() []byte {
	if r == nil || r.decoded == nil {
		return nil
	}
	return r.decoded.Output()
}

// FileNotifyInformation validates every record before returning any of them.
// The returned views remain valid until the owning Response is closed.
func (r *ChangeNotifyResponse) FileNotifyInformation() ([]wire.FileNotifyInformationDecoder, error) {
	if r == nil || r.decoded == nil || r.request.command != wire.SMB2_CHANGE_NOTIFY {
		return nil, errors.New("protocol: payload accessor does not match CHANGE_NOTIFY request")
	}
	output := r.Output()
	if len(output) == 0 {
		return nil, nil
	}
	entries := make([]wire.FileNotifyInformationDecoder, 0, 1)
	for len(output) != 0 {
		entry, err := decodePayload[wire.FileNotifyInformationDecoder](output, wire.SMB2_CHANGE_NOTIFY, "file notify information format")
		if err != nil {
			return nil, err
		}
		if r.request.notifyFlags&wire.SMB2_WATCH_TREE == 0 && strings.ContainsAny(entry.FileName(), `/\\`) {
			return nil, invalidResponse(wire.SMB2_CHANGE_NOTIFY, "invalid file notify information name")
		}
		entries = append(entries, entry)
		next := entry.NextEntryOffset()
		if next == 0 {
			break
		}
		output = output[next:]
	}
	return entries, nil
}

// Output returns the payload from the validated response envelope without
// validating its contents. The read-only view is valid until Response.Close.
func (r *IoctlResponse) Output() []byte {
	if r == nil || r.decoded == nil {
		return nil
	}
	return r.decoded.Output()
}

func (r *IoctlResponse) OutputCount() uint32 {
	if r == nil || r.decoded == nil {
		return 0
	}
	return r.decoded.OutputCount()
}

func (r *IoctlResponse) CtlCode() uint32 {
	if r == nil || r.decoded == nil {
		return 0
	}
	return r.decoded.CtlCode()
}

func (r *IoctlResponse) requireCode(codes ...uint32) error {
	if r != nil && r.decoded != nil && r.request.command == wire.SMB2_IOCTL {
		if slices.Contains(codes, r.request.ctlCode) {
			return nil
		}
	}
	return errors.New("protocol: payload accessor does not match IOCTL request")
}

// FileStandardInformation returns a validated, read-only payload decoder valid until Close.
func (r *QueryInfoResponse) FileStandardInformation() (wire.FileStandardInformationDecoder, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_FILE, wire.FileStandardInformation); err != nil {
		return nil, err
	}
	return decodePayload[wire.FileStandardInformationDecoder](r.Output(), wire.SMB2_QUERY_INFO, "query info response format")
}

// FileBasicInformation returns a validated, read-only payload decoder valid until Close.
func (r *QueryInfoResponse) FileBasicInformation() (wire.FileBasicInformationDecoder, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_FILE, wire.FileBasicInformation); err != nil {
		return nil, err
	}
	return decodePayload[wire.FileBasicInformationDecoder](r.Output(), wire.SMB2_QUERY_INFO, "query info response format")
}

// FileAttributeTagInformation returns a validated decoder valid until Close.
func (r *QueryInfoResponse) FileAttributeTagInformation() (wire.FileAttributeTagInformationDecoder, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_FILE, wire.FileAttributeTagInformation); err != nil {
		return nil, err
	}
	return decodePayload[wire.FileAttributeTagInformationDecoder](r.Output(), wire.SMB2_QUERY_INFO, "attribute tag information")
}

// FileNetworkOpenInformation returns a validated, read-only payload decoder valid until Close.
func (r *QueryInfoResponse) FileNetworkOpenInformation() (wire.FileNetworkOpenInformationDecoder, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_FILE, wire.FileNetworkOpenInformation); err != nil {
		return nil, err
	}
	return decodePayload[wire.FileNetworkOpenInformationDecoder](r.Output(), wire.SMB2_QUERY_INFO, "query info response format")
}

// FileFsFullSizeInformation returns a validated, read-only payload decoder valid until Close.
func (r *QueryInfoResponse) FileFsFullSizeInformation() (wire.FileFsFullSizeInformationDecoder, error) {
	if err := r.requireClass(wire.SMB2_0_INFO_FILESYSTEM, wire.FileFsFullSizeInformation); err != nil {
		return nil, err
	}
	return decodePayload[wire.FileFsFullSizeInformationDecoder](r.Output(), wire.SMB2_QUERY_INFO, "query info response format")
}

// SymbolicLinkReparseData returns a validated, read-only payload decoder valid until Close.
func (r *IoctlResponse) SymbolicLinkReparseData() (wire.SymbolicLinkReparseDataBufferDecoder, error) {
	if err := r.requireCode(wire.FSCTL_GET_REPARSE_POINT); err != nil {
		return nil, err
	}
	return decodePayload[wire.SymbolicLinkReparseDataBufferDecoder](r.Output(), wire.SMB2_IOCTL, "symbolic link response data buffer format")
}

// SrvRequestResumeKey returns a validated, read-only payload decoder valid until Close.
func (r *IoctlResponse) SrvRequestResumeKey() (wire.SrvRequestResumeKeyResponseDecoder, error) {
	if err := r.requireCode(wire.FSCTL_SRV_REQUEST_RESUME_KEY); err != nil {
		return nil, err
	}
	return decodePayload[wire.SrvRequestResumeKeyResponseDecoder](r.Output(), wire.SMB2_IOCTL, "srv request resume key response format")
}

// SrvCopychunk returns a validated, read-only payload decoder valid until Close.
func (r *IoctlResponse) SrvCopychunk() (wire.SrvCopychunkResponseDecoder, error) {
	if err := r.requireCode(wire.FSCTL_SRV_COPYCHUNK, wire.FSCTL_SRV_COPYCHUNK_WRITE); err != nil {
		return nil, err
	}
	decoded, err := decodePayload[wire.SrvCopychunkResponseDecoder](r.Output(), wire.SMB2_IOCTL, "srv copy chunk response format")
	if err != nil {
		return nil, err
	}
	if r.request.hasCopyTotal && uint64(decoded.TotalBytesWritten()) != r.request.copyTotal {
		return nil, invalidResponse(wire.SMB2_IOCTL, "srv copy chunk total bytes written does not match requested total")
	}
	return decoded, nil
}

// describeQueryRequest captures the actual encoded limit, including custom
// wire.Packet implementations passed to Request.Append.
func describeQueryRequest(command wire.Command, body []byte) (payloadRequest, error) {
	if command == wire.SMB2_QUERY_INFO {
		r := wire.QueryInfoRequestDecoder(body)
		if r.IsInvalid() {
			return payloadRequest{}, errors.New("protocol: invalid encoded query info request")
		}
		return payloadRequest{command: command, infoType: r.InfoType(), infoClass: r.FileInfoClass(), additionalInfo: r.AdditionalInformation(), maxOutput: r.OutputBufferLength()}, nil
	}
	r := wire.QueryDirectoryRequestDecoder(body)
	if r.IsInvalid() {
		return payloadRequest{}, errors.New("protocol: invalid encoded query directory request")
	}
	return payloadRequest{command: command, infoClass: r.FileInfoClass(), maxOutput: r.OutputBufferLength()}, nil
}
