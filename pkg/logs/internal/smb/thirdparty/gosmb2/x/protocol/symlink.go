package protocol

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	pathpkg "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/path"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

func (req *Request) resolveSymlink(ctx context.Context, name string, rerr *ResponseError, data []byte) (string, error) {
	if len(data) == 0 {
		return req.querySymlinkTarget(ctx, name, rerr)
	}
	d := wire.SymbolicLinkErrorResponseDecoder(data)
	if d.IsInvalid() {
		return "", invalidResponse(wire.SMB2_CREATE, "broken symbolic link error response format")
	}
	ud, suffix := d.SplitUnparsedPath(name)
	if ud == "" && suffix == "" {
		return "", invalidResponse(wire.SMB2_CREATE, "broken symbolic link error response format")
	}
	return req.resolveSymlinkTarget(name, ud, d.SubstituteName(), suffix, d.Flags(), rerr)
}

// querySymlinkTarget obtains the target when a server (notably macOS) returns
// STATUS_STOPPED_ON_SYMLINK without error data. Probes never follow links:
// if an ancestor stops CREATE, shorten the path until that link can be opened.
func (req *Request) querySymlinkTarget(ctx context.Context, name string, rerr *ResponseError) (string, error) {
	parts := pathpkg.SplitAll(name)
	for count := len(parts); count > 0; count-- {
		linkPath := pathpkg.Join(parts[:count]...)
		probe := req.tc.Request().
			Create(linkPath, wire.FILE_READ_ATTRIBUTES, wire.FILE_OPEN, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
			Ioctl(wire.FSCTL_GET_REPARSE_POINT, nil, 16*1024).
			Close()
		res, err := probe.Do(ctx)
		if err != nil {
			stopped := responseErrorAt(err, 0)
			if stopped != nil && erref.NtStatus(stopped.Code) == erref.STATUS_STOPPED_ON_SYMLINK && continuationSafe(err, probe.pkts) {
				continue
			}
			return "", err
		}
		defer res.Close()
		ioctl, err := res.Ioctl(1)
		if err != nil {
			return "", err
		}
		link, err := ioctl.SymbolicLinkReparseData()
		if err != nil {
			return "", err
		}
		suffix, ok := pathpkg.CutPrefix(name, linkPath)
		if !ok {
			return "", invalidResponse(wire.SMB2_CREATE, "invalid symbolic link path prefix")
		}
		return req.resolveSymlinkTarget(name, linkPath, link.SubstituteName(), suffix, link.Flags(), rerr)
	}
	return "", rerr
}

func (req *Request) resolveSymlinkTarget(name, linkPath, target, suffix string, flags uint32, rerr *ResponseError) (string, error) {
	if flags&wire.SYMLINK_FLAG_RELATIVE != 0 {
		resolved, err := pathpkg.ResolveRelativeSymlink(linkPath, pathpkg.ToSMBPath(target), suffix)
		if err != nil {
			return "", invalidResponse(wire.SMB2_CREATE, err.Error())
		}
		if utf16le.EncodedStringLen(resolved) > math.MaxUint16 {
			return "", errors.New("protocol: resolved symbolic link path exceeds uint16")
		}
		return resolved, nil
	}
	resolved, ok := pathpkg.NormalizeSymlinkUNC(target + suffix)
	if !ok {
		return "", invalidResponse(wire.SMB2_CREATE, "symbolic link target is not a valid UNC path")
	}
	if utf16le.EncodedStringLen(resolved) > math.MaxUint16 {
		return "", errors.New("protocol: resolved symbolic link path exceeds uint16")
	}
	server, share, rest, ok := pathpkg.SplitSymlinkUNC(resolved)
	if !ok {
		return "", invalidResponse(wire.SMB2_CREATE, "symbolic link target is not a UNC path")
	}
	if strings.EqualFold(server, req.tc.serverName) && strings.EqualFold(share, req.tc.shareName) {
		return rest, nil
	}
	return "", &CrossShareSymlinkError{
		Path: req.tc.uncPath(name), Target: target, Relative: false,
		UnparsedPath: suffix, ResolvedPath: resolved, err: rerr,
	}
}
