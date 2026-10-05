// Original: src/os/path.go and src/os/removeall_noat.go
//
// Copyright 2009 The Go Authors. All rights reserved.
// Portions Copyright 2016 Hiroshi Ioka. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// Original: src/path/filepath/match.go
//
// Copyright 2010 The Go Authors. All rights reserved.
// Portions Copyright 2021 Hiroshi Ioka. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package smb2

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/directory"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	pathpkg "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/path"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/security"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

func fileAttributesFromPerm(perm os.FileMode) uint32 {
	attrs := uint32(wire.FILE_ATTRIBUTE_NORMAL)
	if perm&0o200 == 0 {
		attrs |= wire.FILE_ATTRIBUTE_READONLY
	}
	return attrs
}

// Share represents a SMB tree connection with VFS interface.
type Share struct {
	treeConn         *protocol.Tree
	aaplCapabilities uint64
	closeMu          sync.Mutex
	unmounted        bool
}

// Unmount disconnects the current SMB tree and cached DFS trees.
// The Client retains their sessions until Client.Close.
//
// While [MS-SMB2] 3.3.5.8 guarantees that the server will clean up and close
// any remaining opens upon receiving TREE_DISCONNECT, [MS-SMB2] 3.2.4.22
// specifies that the client MUST close all open files on the tree connect
// beforehand. Callers should properly manage and close their open file
// resources rather than relying on server teardown, as abrupt disconnects
// can discard write errors or invalidate active handles.
func (fs *Share) Unmount(ctx context.Context) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return errors.New("smb2: nil Share")
	}
	fs.closeMu.Lock()
	defer fs.closeMu.Unlock()
	if fs.unmounted {
		return nil
	}
	if fs.treeConn != nil {
		if err := fs.treeConn.Disconnect(ctx); err != nil {
			return err
		}
	}
	fs.unmounted = true
	return nil
}

func (fs *Share) Create(ctx context.Context, name string) (*File, error) {
	if ctx == nil {
		panic("nil context")
	}
	return fs.OpenFile(ctx, name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (fs *Share) Open(ctx context.Context, name string) (*File, error) {
	if ctx == nil {
		panic("nil context")
	}
	return fs.OpenFile(ctx, name, os.O_RDONLY, 0)
}

func (fs *Share) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (*File, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	var err error
	name, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return nil, err
	}

	var access uint32
	switch flag & (os.O_RDONLY | os.O_WRONLY | os.O_RDWR) {
	case os.O_RDONLY:
		access = wire.GENERIC_READ
	case os.O_WRONLY:
		access = wire.GENERIC_WRITE
	case os.O_RDWR:
		access = wire.GENERIC_READ | wire.GENERIC_WRITE
	}
	if flag&os.O_TRUNC != 0 {
		access |= wire.GENERIC_WRITE
	}

	var createmode uint32
	switch {
	case flag&(os.O_CREATE|os.O_EXCL) == (os.O_CREATE | os.O_EXCL):
		createmode = wire.FILE_CREATE
	case flag&(os.O_CREATE|os.O_TRUNC) == (os.O_CREATE | os.O_TRUNC):
		createmode = wire.FILE_OVERWRITE_IF
	case flag&os.O_CREATE == os.O_CREATE:
		createmode = wire.FILE_OPEN_IF
	case flag&os.O_TRUNC == os.O_TRUNC:
		createmode = wire.FILE_OVERWRITE
	default:
		createmode = wire.FILE_OPEN
	}

	var createoptions uint32
	if flag&(os.O_CREATE|os.O_EXCL) == (os.O_CREATE | os.O_EXCL) {
		createoptions |= wire.FILE_OPEN_REPARSE_POINT
	}
	if flag&os.O_SYNC != 0 {
		createoptions |= wire.FILE_WRITE_THROUGH
	}

	req := fs.Request().WithFollowSymlinks(true).
		Create(name, access, createmode, createoptions, fileAttributesFromPerm(perm), wire.QueryOnDiskIDRequest{})
	res, err := req.Do(ctx)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	defer res.Close()

	r, err := res.Create(0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	f := fs.newFile(r, name)
	f.appendMode = flag&os.O_APPEND != 0
	// Record read access so copyFile can choose an IOCTL supported by this
	// handle ([MS-SMB2] 2.2.31, 3.2.5.15.6).
	f.readAccess = access&(wire.FILE_READ_DATA|wire.GENERIC_READ|wire.GENERIC_ALL) != 0
	return f, nil
}

func (fs *Share) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		Create(name, wire.FILE_WRITE_ATTRIBUTES, wire.FILE_CREATE, wire.FILE_DIRECTORY_FILE, fileAttributesFromPerm(perm)).
		Close().
		Do(ctx)
	if err != nil {
		return &os.PathError{Op: "mkdir", Path: name, Err: err}
	}
	res.Close()
	return nil
}

func (fs *Share) Remove(ctx context.Context, name string) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}

	// [MS-SMB2] 2.2.13 defines a zero-length CREATE file name as a request
	// to open the root of the share, so an empty name must not reach CREATE.
	if len(name) == 0 {
		return os.ErrInvalid
	}

	remove := fs.Request().WithFollowSymlinks(true).
		Create(name, wire.DELETE, wire.FILE_OPEN, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
		SetInfo(wire.SMB2_0_INFO_FILE, wire.FileDispositionInformation, 0, &wire.FileDispositionInformationEncoder{DeletePending: 1}).
		Close()
	res, err := remove.Do(ctx)
	if err != nil {
		if !errors.Is(err, erref.STATUS_ACCESS_DENIED) && !errors.Is(err, erref.STATUS_CANNOT_DELETE) {
			return &os.PathError{Op: "remove", Path: name, Err: err}
		}

		if err := fs.chmod(ctx, nil, name, 0o666, false); err != nil {
			return &os.PathError{Op: "remove", Path: name, Err: err}
		}

		res, err = remove.Do(ctx)
		if err != nil {
			return &os.PathError{Op: "remove", Path: name, Err: err}
		}
	}
	res.Close()

	return nil
}

func (fs *Share) Rename(ctx context.Context, oldpath, newpath string) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	var err error
	oldpath, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(oldpath))
	if err != nil {
		return err
	}
	newpath, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(newpath))
	if err != nil {
		return err
	}

	// [MS-SMB2] 2.2.13 defines a zero-length CREATE file name as a request
	// to open the root of the share, so neither end may name the share root.
	if len(oldpath) == 0 || len(newpath) == 0 {
		return os.ErrInvalid
	}

	rename := &wire.FileRenameInformationType2Encoder{
		ReplaceIfExists: 1,
		RootDirectory:   0,
		FileName:        newpath,
	}
	// [MS-SMB2] 3.2.1.2 defines MaxTransactSize and 3.3.5.21 requires the
	// server to reject a SET_INFO whose BufferLength exceeds it. Reject an
	// oversized rename locally so no oversized compound request is sent.
	if rename.Size() > fs.maxTransactSize(2) {
		return os.ErrInvalid
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		Create(oldpath, wire.DELETE, wire.FILE_OPEN, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
		SetInfo(wire.SMB2_0_INFO_FILE, wire.FileRenameInformation, 0, rename).
		Close().
		Do(ctx)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	res.Close()

	return nil
}

func (fs *Share) Readlink(ctx context.Context, name string) (string, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return "", os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return "", err
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		Create(name, wire.FILE_READ_ATTRIBUTES, wire.FILE_OPEN, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
		Ioctl(wire.FSCTL_GET_REPARSE_POINT, nil, maxSingleCreditPayloadSize).
		Close().
		Do(ctx)
	if err != nil {
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}
	defer res.Close()

	r1, err := res.Ioctl(1)
	if err != nil {
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}

	r, err := r1.SymbolicLinkReparseData()
	if err != nil {
		return "", &os.PathError{Op: "readlink", Path: name, Err: err}
	}

	return r.SubstituteName(), nil
}

// Symlink mimics os.Symlink.
// This API should work on latest Windows, latest MacOS, and Samba 4.21 or later.
// Also there is a restriction on target pathname. Generally, a pathname begins with leading backslash (e.g `\dir\name`) can be interpreted as two ways.
// On windows, it is evaluated as a relative path, on other systems, it is evaluated as an absolute path.
// This implementation always assumes that format is absolute path. So, if you know the target server is Windows, you should avoid that format.
// If you want to use an absolute target path on windows, you can use `C:\dir\name` format instead.
func (fs *Share) Symlink(ctx context.Context, target, linkpath string) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	target = pathpkg.NormalizeSymlinkTarget(pathpkg.ToSMBPath(target))
	if len(target) == 0 {
		return os.ErrInvalid
	}

	linkpath, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(linkpath))
	if err != nil {
		return err
	}

	substituteName, printName, relative, ok := pathpkg.BuildSymlinkReparseNames(target)
	if !ok {
		return os.ErrInvalid
	}

	rdbuf := &wire.SymbolicLinkReparseDataBuffer{
		SubstituteName: substituteName,
		PrintName:      printName,
	}
	if relative {
		rdbuf.Flags = wire.SYMLINK_FLAG_RELATIVE
	}

	// [MS-FSCC] 2.3.82 rejects FSCTL_SET_REPARSE_POINT input buffers over
	// 16,384 bytes, including the common header. The symbolic-link layout
	// is defined in [MS-FSCC] 2.1.2.4.
	if rdbuf.Size() > maxReparseDataBufferSize {
		return os.ErrInvalid
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		Create(linkpath, wire.FILE_WRITE_ATTRIBUTES|wire.DELETE, wire.FILE_CREATE, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
		Ioctl(wire.FSCTL_SET_REPARSE_POINT, rdbuf, 0).
		Close().
		Do(ctx)
	if err != nil {
		if cerr, ok := errors.AsType[*protocol.CompoundResponseError](err); ok && cerr.OpError(0) == nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
			defer cancel()
			err = errors.Join(err, fs.Remove(cleanupCtx, linkpath))
		}
		return &os.LinkError{Op: "symlink", Old: target, New: linkpath, Err: err}
	}
	res.Close()

	return nil
}

func (fs *Share) ReadFile(ctx context.Context, filename string) ([]byte, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	filename, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(filename))
	if err != nil {
		return nil, err
	}

	firstReq := fs.Request().WithFollowSymlinks(true).
		Create(filename, wire.GENERIC_READ, wire.FILE_OPEN, wire.FILE_NON_DIRECTORY_FILE, wire.FILE_ATTRIBUTE_NORMAL).
		Read(maxSingleCreditPayloadSize, 0)
	res, err := firstReq.Do(ctx)
	var (
		overflowData []byte
		isOverflow   bool
	)
	if err != nil {
		// An empty file is not an error: servers report STATUS_END_OF_FILE on
		// the READ of a compound CREATE+READ when the file has no data
		// ([MS-SMB2] 2.2.42). Treat it as success with no content.
		readErr := protocol.ResponseErrorAt(err, 1)
		if cerr, ok := errors.AsType[*protocol.CompoundResponseError](err); ok && cerr.OpError(0) != nil {
			// The opening CREATE failed, so op 1 was not a completed READ.
			readErr = nil
		}
		if readErr != nil {
			switch erref.NtStatus(readErr.Code) {
			case erref.STATUS_END_OF_FILE:
				return []byte{}, nil
			case erref.STATUS_BUFFER_OVERFLOW:
				isOverflow = true
				if data, ok := protocol.BufferOverflowData(readErr); ok {
					overflowData = append([]byte(nil), data...)
				}
			}
		}
		if !isOverflow {
			return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
		}
	}

	var (
		f         *File
		createRes wire.CreateResponseDecoder
		first     []byte
	)
	if isOverflow {
		secondReq := fs.Request().WithFollowSymlinks(true).
			Create(filename, wire.GENERIC_READ, wire.FILE_OPEN, wire.FILE_NON_DIRECTORY_FILE, wire.FILE_ATTRIBUTE_NORMAL)
		res2, err := secondReq.Do(ctx)
		if err != nil {
			return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
		}
		defer res2.Close()

		createR, err := res2.Create(0)
		if err != nil {
			return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
		}
		f = fs.newFile(createR, res2.ResolvedPath())
		defer f.closeAfterOperation(ctx)
		createRes = createR
		first = overflowData
	} else {
		defer res.Close()
		createR, err := res.Create(0)
		if err != nil {
			return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
		}
		f = fs.newFile(createR, res.ResolvedPath())
		defer f.closeAfterOperation(ctx)
		createRes = createR
		readRes, err := res.Read(1)
		if err != nil {
			return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
		}
		first = readRes.Data()
	}

	// CREATE reports a size snapshot. Preserve any additional bytes already
	// received if the file grew before the initial READ completed.
	size := max(createRes.EndofFile(), int64(len(first)))
	data, err := makeReadFileBuffer(size)
	if err != nil {
		return nil, &os.PathError{Op: "readfile", Path: filename, Err: err}
	}
	copy(data, first)

	if off := len(first); off < len(data) {
		// readAt splits the remaining range by the effective READ limit and
		// reserves credits for each request before sending it.
		n, readErr := fs.readAt(ctx, f.fd, data[off:], int64(off))
		if readErr == io.EOF || (readErr == nil && n < len(data)-off) {
			readErr = io.ErrUnexpectedEOF
		}
		if readErr != nil {
			return data[:off+n], &os.PathError{Op: "readfile", Path: filename, Err: readErr}
		}
	}

	return data, nil
}

func makeReadFileBuffer(size int64) (buf []byte, err error) {
	// Keep recovery limited to allocation: Go's slice-size limit depends on
	// the architecture and can be smaller than MaxInt. Actual memory
	// exhaustion remains a runtime failure, as with other whole-file reads.
	defer func() {
		if recover() != nil {
			err = errors.New("smb2: file size exceeds byte slice limit")
		}
	}()
	return make([]byte, size), nil
}

func (fs *Share) WriteFile(ctx context.Context, filename string, data []byte, perm os.FileMode) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	filename, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(filename))
	if err != nil {
		return err
	}

	attrs := fileAttributesFromPerm(perm)

	maxWriteSize := fs.maxWriteSize(2)

	if len(data) <= maxWriteSize { // first path
		res, err := fs.Request().WithFollowSymlinks(true).
			Create(filename, wire.GENERIC_WRITE, wire.FILE_OVERWRITE_IF, wire.FILE_NON_DIRECTORY_FILE, attrs).
			Write(data, 0).
			Close().
			Do(ctx)
		if err != nil {
			return &os.PathError{Op: "writefile", Path: filename, Err: err}
		}
		defer res.Close()

		writeR, err := res.Write(1)
		if err != nil {
			return &os.PathError{Op: "writefile", Path: filename, Err: err}
		}
		count := writeR.Count()
		if uint64(count) < uint64(len(data)) {
			return &os.PathError{Op: "writefile", Path: filename, Err: io.ErrShortWrite}
		}
		return nil
	}

	f, err := fs.OpenFile(ctx, filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok {
			err = pathErr.Err
		}
		return &os.PathError{Op: "writefile", Path: filename, Err: err}
	}

	_, err = f.Write(ctx, data)
	return errors.Join(err, f.closeAfterOperation(ctx))
}

func (fs *Share) Truncate(ctx context.Context, name string, size int64) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}

	if err := fs.truncate(ctx, nil, name, size); err != nil {
		return &os.PathError{Op: "truncate", Path: name, Err: err}
	}
	return nil
}

func (fs *Share) Chtimes(ctx context.Context, name string, atime time.Time, mtime time.Time) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}

	if err := fs.chtimes(ctx, nil, name, atime, mtime); err != nil {
		return &os.PathError{Op: "chtimes", Path: name, Err: err}
	}
	return nil
}

func (fs *Share) Chmod(ctx context.Context, name string, mode os.FileMode) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}

	if err := fs.chmod(ctx, nil, name, mode, true); err != nil {
		return &os.PathError{Op: "chmod", Path: name, Err: err}
	}
	return nil
}

func (fs *Share) newFile(r wire.CreateResponseDecoder, name string) *File {
	fd := r.FileId().Decode()

	f := &File{
		fs:    fs,
		fd:    fd,
		name:  name,
		isDir: r.FileAttributes()&wire.FILE_ATTRIBUTE_DIRECTORY != 0,
	}

	if identity := r.QueryOnDiskID(); identity != nil {
		f.fileId = identity.DiskFileId()
		f.volumeId = identity.VolumeId()
		f.hasIdentity = true
	}

	runtime.SetFinalizer(f, func(f *File) {
		if f == nil {
			return
		}
		if f.closed.CompareAndSwap(false, true) {
			ctx, cancel := context.WithTimeout(context.Background(), clientCleanupTimeout)
			defer cancel()
			f.fs.closeFile(ctx, f.fd)
		}
	})

	return f
}

func (fs *Share) truncate(ctx context.Context, fd *wire.FileId, name string, size int64) error {
	if size < 0 {
		return os.ErrInvalid
	}

	req := fs.Request().WithFollowSymlinks(true)
	if fd != nil {
		req.WithFileID(*fd)
	} else {
		req.Create(name, wire.FILE_WRITE_DATA, wire.FILE_OPEN, wire.FILE_NON_DIRECTORY_FILE, wire.FILE_ATTRIBUTE_NORMAL)
	}

	req.SetInfo(wire.SMB2_0_INFO_FILE, wire.FileEndOfFileInformation, 0, &wire.FileEndOfFileInformationEncoder{EndOfFile: size})

	if fd == nil {
		req.Close()
	}

	res, err := req.Do(ctx)
	if err != nil {
		return err
	}
	res.Close()
	return nil
}

func (fs *Share) chtimes(ctx context.Context, fd *wire.FileId, name string, atime time.Time, mtime time.Time) error {
	accessTime, ok := wire.TimeToFiletime(atime)
	if !ok {
		return os.ErrInvalid
	}
	writeTime, ok := wire.TimeToFiletime(mtime)
	if !ok {
		return os.ErrInvalid
	}

	req := fs.Request().WithFollowSymlinks(true)
	if fd != nil {
		req.WithFileID(*fd)
	} else {
		req.Create(name, wire.FILE_WRITE_ATTRIBUTES, wire.FILE_OPEN, 0, wire.FILE_ATTRIBUTE_NORMAL)
	}

	req.SetInfo(wire.SMB2_0_INFO_FILE, wire.FileBasicInformation, 0, &wire.FileBasicInformationEncoder{
		LastAccessTime: accessTime,
		LastWriteTime:  writeTime,
	})

	if fd == nil {
		req.Close()
	}

	res, err := req.Do(ctx)
	if err != nil {
		return err
	}
	res.Close()
	return nil
}

func (fs *Share) chmod(ctx context.Context, fd *wire.FileId, name string, mode os.FileMode, followSymlink bool) error {
	req1 := fs.Request().WithFollowSymlinks(true)
	if fd != nil {
		req1.WithFileID(*fd).QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileBasicInformation, 0, 40)
	} else {
		var options uint32
		if !followSymlink {
			options = wire.FILE_OPEN_REPARSE_POINT
		}
		req1.Create(name, wire.FILE_READ_ATTRIBUTES|wire.FILE_WRITE_ATTRIBUTES, wire.FILE_OPEN, options, wire.FILE_ATTRIBUTE_NORMAL)
	}

	// 1st RTT: CREATE or QUERY_INFO for an existing handle.
	res1, err := req1.Do(ctx)
	if err != nil {
		return err
	}
	defer res1.Close()

	var targetFd wire.FileId
	var attrs uint32
	if fd != nil {
		targetFd = *fd
		queryRes, err := res1.QueryInfo(0)
		if err != nil {
			return err
		}
		base, err := queryRes.FileBasicInformation()
		if err != nil {
			return err
		}
		attrs = base.FileAttributes()
	} else {
		createRes, err := res1.Create(0)
		if err != nil {
			return err
		}
		targetFd = createRes.FileId().Decode()
		attrs = createRes.FileAttributes()
	}

	attrs = computeChmodAttrs(attrs, mode)

	// 2nd RTT: SET_INFO
	// Keep SET_INFO separate from CLOSE. Some servers close the handle while
	// processing a related SET_INFO+CLOSE compound request for read-only files.
	res2, err := fs.Request().WithFollowSymlinks(true).
		WithFileID(targetFd).
		SetInfo(wire.SMB2_0_INFO_FILE, wire.FileBasicInformation, 0, &wire.FileBasicInformationEncoder{FileAttributes: attrs}).
		Do(ctx)
	if err != nil {
		if fd == nil {
			// This internal handle has no caller to retry cleanup after cancellation.
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
			defer cancel()
			_ = fs.closeFile(closeCtx, targetFd)
		}
		return err
	}
	res2.Close()

	if fd == nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
		defer cancel()
		if err := fs.closeFile(closeCtx, targetFd); err != nil {
			return err
		}
		return ctx.Err()
	}

	return nil
}

func (fs *Share) flush(ctx context.Context, fd wire.FileId) error {
	res, err := fs.Request().WithFollowSymlinks(true).WithFileID(fd).Flush().Do(ctx)
	if err != nil {
		return err
	}
	res.Close()

	return nil
}

func (fs *Share) closeFile(ctx context.Context, fd wire.FileId) error {
	return fs.treeConn.CloseFile(ctx, fd)
}

func (fs *Share) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return nil, err
	}

	fi, err := fs.stat(ctx, nil, name)
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: name, Err: err}
	}
	return fi, nil
}

func (fs *Share) Lstat(ctx context.Context, name string) (os.FileInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return nil, err
	}

	fi, err := fs.lstat(ctx, name)
	if err != nil {
		return nil, &os.PathError{Op: "lstat", Path: name, Err: err}
	}
	return fi, nil
}

func (fs *Share) statPath(ctx context.Context, name string, createOptions uint32) (os.FileInfo, error) {
	req := fs.Request().WithFollowSymlinks(true).
		Create(name, wire.FILE_READ_ATTRIBUTES, wire.FILE_OPEN, createOptions, wire.FILE_ATTRIBUTE_NORMAL, wire.QueryOnDiskIDRequest{}).
		QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileAttributeTagInformation, 0, 8).
		Close()
	res, err := req.Do(ctx)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	r, err := res.Create(0)
	if err != nil {
		return nil, err
	}
	stat := newFileStatFromCreateResponse(r, name)
	if err := applyAttributeTag(stat, res, 1); err != nil {
		return nil, err
	}
	return stat, nil
}

func (fs *Share) stat(ctx context.Context, fd *wire.FileId, name string) (os.FileInfo, error) {
	if fd == nil {
		return fs.statPath(ctx, name, 0)
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		WithFileID(*fd).
		QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileNetworkOpenInformation, 0, 56).
		Do(ctx)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	queryRes, err := res.QueryInfo(0)
	if err != nil {
		return nil, err
	}
	info, err := queryRes.FileNetworkOpenInformation()
	if err != nil {
		return nil, err
	}

	stat := newFileStatFromFileNetworkOpenInformation(info, name)
	if stat.FileAttributes&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		tagRes, err := fs.Request().WithFileID(*fd).
			QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileAttributeTagInformation, 0, 8).Do(ctx)
		if err != nil {
			return nil, err
		}
		defer tagRes.Close()
		if err := applyAttributeTag(stat, tagRes, 0); err != nil {
			return nil, err
		}
	}
	return stat, nil
}

func applyAttributeTag(stat *FileStat, res *protocol.Response, index int) error {
	query, err := res.QueryInfo(index)
	if err != nil {
		return err
	}
	info, err := query.FileAttributeTagInformation()
	if err != nil {
		return err
	}
	stat.FileAttributes = info.FileAttributes()
	if stat.FileAttributes&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		stat.ReparseTag = info.ReparseTag()
	}
	return nil
}

func (fs *Share) lstat(ctx context.Context, name string) (os.FileInfo, error) {
	return fs.statPath(ctx, name, wire.FILE_OPEN_REPARSE_POINT)
}

func (fs *Share) Statfs(ctx context.Context, name string) (FileFsInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return nil, err
	}

	info, err := fs.statfs(ctx, nil, name)
	if err != nil {
		return nil, &os.PathError{Op: "statfs", Path: name, Err: err}
	}
	return info, nil
}

func (fs *Share) statfs(ctx context.Context, fd *wire.FileId, name string) (FileFsInfo, error) {
	req := fs.Request().WithFollowSymlinks(true)
	idx := 0
	if fd != nil {
		req.WithFileID(*fd)
	} else {
		req.Create(name, wire.FILE_READ_ATTRIBUTES, wire.FILE_OPEN, 0, wire.FILE_ATTRIBUTE_NORMAL)
		idx = 1
	}

	req.QueryInfo(wire.SMB2_0_INFO_FILESYSTEM, wire.FileFsFullSizeInformation, 0, 32)

	if fd == nil {
		req.Close()
	}

	res, err := req.Do(ctx)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	queryRes, err := res.QueryInfo(idx)
	if err != nil {
		return nil, err
	}
	return parseFsFullSizeInfo(queryRes)
}

func (fs *Share) ReadDir(ctx context.Context, dirname string) ([]os.FileInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	var err error
	dirname, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(dirname))
	if err != nil {
		return nil, err
	}

	req := fs.Request().WithFollowSymlinks(true).
		Create(dirname, wire.FILE_READ_DATA|wire.FILE_READ_ATTRIBUTES|wire.READ_CONTROL, wire.FILE_OPEN, wire.FILE_DIRECTORY_FILE, wire.FILE_ATTRIBUTE_NORMAL).
		QueryDir(wire.FileIdBothDirectoryInformation, "*", maxSingleCreditPayloadSize)
	res, err := req.Do(ctx)
	if err != nil {
		// An empty directory is not an error: some servers (e.g. Samba)
		// report STATUS_NO_MORE_FILES or STATUS_NO_SUCH_FILE on the first
		// QUERY_DIRECTORY of a compound CREATE+QUERY_DIRECTORY when the
		// directory has no entries ([MS-FSA] 2.1.5.6.3). Treat it as
		// success with no content.
		if cerr, ok := errors.AsType[*protocol.CompoundResponseError](err); ok && cerr.OpError(0) == nil {
			if rerr, ok := errors.AsType[*protocol.ResponseError](cerr.OpError(1)); ok {
				switch erref.NtStatus(rerr.Code) {
				case erref.STATUS_NO_MORE_FILES, erref.STATUS_NO_SUCH_FILE:
					return []os.FileInfo{}, nil
				}
			}
		}
		return nil, &os.PathError{Op: "readdir", Path: dirname, Err: err}
	}
	defer res.Close()

	createR, err := res.Create(0)
	if err != nil {
		return nil, &os.PathError{Op: "readdir", Path: dirname, Err: err}
	}
	f := fs.newFile(createR, res.ResolvedPath())
	defer f.closeAfterOperation(ctx)

	queryRes, err := res.QueryDir(1)
	if err != nil {
		return nil, &os.PathError{Op: "readdir", Path: dirname, Err: err}
	}
	fis, err := f.readdirAll(ctx, queryRes)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			err = &os.PathError{Op: "readdir", Path: dirname, Err: err}
		}
	}
	return fis, err
}

func (fs *Share) readdir(ctx context.Context, fd wire.FileId, pattern string, restart bool) ([]os.FileInfo, error) {
	read := directory.ReadPage[os.FileInfo]
	if restart {
		read = directory.RestartPage[os.FileInfo]
	}
	return read(ctx, fs.Request, fd, pattern, func(entry wire.FileIdBothDirectoryInformationDecoder) os.FileInfo {
		return newFileStatFromFileIdBothDirectoryInformation(entry, entry.FileName())
	})
}

// WithContext returns an io/fs.FS adapter using ctx for its operations.
// After a CREATE has been sent, cancellation waits for the final responses
// and handle cleanup so
// a successful open cannot leak. A server that does not finish the request can
// delay cancellation until the connection is closed.
func (fs *Share) WithContext(ctx context.Context) interface {
	iofs.FS
	iofs.StatFS
	iofs.ReadFileFS
	iofs.ReadDirFS
	iofs.GlobFS
	iofs.ReadLinkFS
	iofs.SubFS
} {
	if ctx == nil {
		panic("nil context")
	}
	return &boundShare{share: fs, ctx: ctx}
}

func (fs *Share) copyFile(ctx context.Context, srcFd, dstFd wire.FileId, srcName, dstName string, srcOffset, dstOffset int64, dstReadAccess bool) (supported bool, n int64, err error) {
	// [MS-SMB2] 2.2.31: FSCTL_SRV_COPYCHUNK requires FILE_READ_DATA on the
	// destination handle, while FSCTL_SRV_COPYCHUNK_WRITE only requires write
	// access. Choose the strongest code the destination handle permits.
	copyCtlCode := uint32(wire.FSCTL_SRV_COPYCHUNK_WRITE)
	if dstReadAccess {
		copyCtlCode = wire.FSCTL_SRV_COPYCHUNK
	}

	if srcOffset < 0 || dstOffset < 0 {
		return true, 0, os.ErrInvalid
	}

	// Some servers use SourceOffset as the destination offset in COPYCHUNK.
	// Equal offsets preserve the fast path without depending on server identity.
	if srcOffset != dstOffset {
		return false, 0, nil
	}

	req := &wire.IoctlRequest{
		FileId:            srcFd,
		CtlCode:           wire.FSCTL_SRV_REQUEST_RESUME_KEY,
		OutputOffset:      0,
		OutputCount:       0,
		MaxInputResponse:  0,
		MaxOutputResponse: 32,
		Flags:             wire.SMB2_0_IOCTL_IS_FSCTL,
	}

	res, err := fs.Request().WithFileID(srcFd).Append(req).Do(ctx)
	if err != nil {
		// [MS-SMB2] 3.3.5.15 recommends these statuses for FSCTLs not allowed
		// on the server or unsupported by the filesystem, respectively.
		// The resume key request has not copied any bytes, so fallback is safe.
		if errors.Is(err, erref.STATUS_NOT_SUPPORTED) || errors.Is(err, erref.STATUS_INVALID_DEVICE_REQUEST) {
			return false, 0, nil
		}

		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}
	defer res.Close()

	ioctlRes, err := res.Ioctl(0)
	if err != nil {
		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}
	sr, err := ioctlRes.SrvRequestResumeKey()
	if err != nil {
		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}

	infoRes, err := fs.Request().WithFollowSymlinks(true).WithFileID(srcFd).
		QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileStandardInformation, 0, 24).
		Do(ctx)
	if err != nil {
		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}
	defer infoRes.Close()

	queryRes, err := infoRes.QueryInfo(0)
	if err != nil {
		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}
	info, err := queryRes.FileStandardInformation()
	if err != nil {
		return true, 0, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
	}

	end := info.EndOfFile()
	off := srcOffset
	woff := dstOffset

	if end <= off {
		return true, 0, nil
	}

	remains := end - off
	if remains > math.MaxInt64-dstOffset {
		return true, 0, os.ErrInvalid
	}
	// [MS-SMB2] 2.2.31.1.1 defines these as offsets from each file's start.
	// Nonnegative offsets, a nonnegative EndOfFile, and the full-range check
	// keep every chunk offset and the final file position within int64.

	var srvChunks [16]wire.SrvCopychunk

	for {
		var reqChunks []wire.SrvCopychunk

		if remains < clientMaxCopyTotalSize {
			nchunks := remains / clientMaxCopyChunkSize
			for i := int64(0); i < nchunks; i++ {
				srvChunks[i] = wire.SrvCopychunk{
					SourceOffset: off + i*clientMaxCopyChunkSize,
					TargetOffset: woff + i*clientMaxCopyChunkSize,
					Length:       clientMaxCopyChunkSize,
				}
			}

			remains %= clientMaxCopyChunkSize
			if remains != 0 {
				srvChunks[nchunks] = wire.SrvCopychunk{
					SourceOffset: off + nchunks*clientMaxCopyChunkSize,
					TargetOffset: woff + nchunks*clientMaxCopyChunkSize,
					Length:       uint32(remains),
				}
				nchunks++
				remains = 0
			}

			reqChunks = srvChunks[:nchunks]
		} else {
			for i := range int64(16) {
				srvChunks[i] = wire.SrvCopychunk{
					SourceOffset: off + i*clientMaxCopyChunkSize,
					TargetOffset: woff + i*clientMaxCopyChunkSize,
					Length:       clientMaxCopyChunkSize,
				}
			}

			reqChunks = srvChunks[:16]
			remains -= clientMaxCopyTotalSize
			off += clientMaxCopyTotalSize
			woff += clientMaxCopyTotalSize
		}

		scc := &wire.SrvCopychunkCopy{
			Chunks: reqChunks,
		}

		copy(scc.SourceKey[:], sr.ResumeKey())

		cReq := &wire.IoctlRequest{
			FileId:            dstFd,
			CtlCode:           copyCtlCode,
			OutputOffset:      0,
			OutputCount:       0,
			MaxInputResponse:  0,
			MaxOutputResponse: 24,
			Flags:             wire.SMB2_0_IOCTL_IS_FSCTL,
			Input:             scc,
		}

		copyRes, err := fs.Request().WithFileID(dstFd).Append(cReq).Do(ctx)
		if err != nil {
			// [MS-SMB2] 3.3.5.15: STATUS_NOT_SUPPORTED is the server-wide
			// "unknown FSCTL" answer and STATUS_INVALID_DEVICE_REQUEST is the
			// filesystem-wide "unsupported FSCTL" answer. Only the WRITE
			// variant may fall back to a buffered copy, and only before any
			// byte was transferred. ACCESS_DENIED is not an unsupported
			// signal and must be surfaced as-is.
			if copyCtlCode == wire.FSCTL_SRV_COPYCHUNK_WRITE && n == 0 &&
				(errors.Is(err, erref.STATUS_NOT_SUPPORTED) || errors.Is(err, erref.STATUS_INVALID_DEVICE_REQUEST)) {
				return false, 0, nil
			}

			return true, n, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: err}
		}

		copyIoctl, decodeErr := copyRes.Ioctl(0)
		if decodeErr != nil {
			copyRes.Close()
			return true, n, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: decodeErr}
		}
		c, decodeErr := copyIoctl.SrvCopychunk()
		written := uint32(0)
		if decodeErr == nil {
			written = c.TotalBytesWritten()
		}
		copyRes.Close()
		if decodeErr != nil {
			return true, n, &os.LinkError{Op: "copy", Old: srcName, New: dstName, Err: decodeErr}
		}

		n += int64(written)

		if remains == 0 {
			return true, n, nil
		}
	}
}

func (fs *Share) readAtChunk(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	m := min(len(b), fs.maxReadSize(0))
	if m == 0 {
		return 0, nil
	}
	job := ioPipelineJob{start: 0, end: m, off: off}
	res, err := fs.Request().Append(fs.makeReadRequest(fd, b, job)).Do(ctx)
	if err != nil {
		return fs.parseReadResponse(b, job, nil, err)
	}
	defer res.Close()
	return fs.parseReadResponse(b, job, res, nil)
}

func (fs *Share) writeAtChunk(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	m := min(len(b), fs.maxWriteSize(0))
	if m == 0 {
		return 0, nil
	}

	req := &wire.WriteRequest{
		Flags:            0,
		Channel:          0,
		RemainingBytes:   0,
		Offset:           uint64(off),
		WriteChannelInfo: nil,
		Data:             b[:m],
		FileId:           fd,
	}

	res, err := fs.Request().Append(req).Do(ctx)
	if err != nil {
		return 0, err
	}
	defer res.Close()

	return parseWriteResponse(res, m)
}

func parseWriteResponse(rp *protocol.Response, requested int) (int, error) {
	r, err := rp.Write(0)
	if err != nil {
		return 0, err
	}
	if r.Count() < uint32(requested) {
		return int(r.Count()), io.ErrShortWrite
	}
	return int(r.Count()), nil
}

func validFileRange(off int64, size int) bool {
	return off >= 0 && (size == 0 || int64(size-1) <= math.MaxInt64-off)
}

func (fs *Share) maxReadSize(companions int) int {
	return fs.treeConn.MaxReadSize(companions)
}

func (fs *Share) maxWriteSize(companions int) int {
	return fs.treeConn.MaxWriteSize(companions)
}

func (fs *Share) maxTransactSize(companions int) int {
	return fs.treeConn.MaxTransactSize(companions)
}

func (fs *Share) ioPipelineDepth() uint { return fs.treeConn.IOPipelineDepth() }

type ioPipelineJob struct {
	start int
	end   int
	off   int64
	req   wire.Packet
}

type ioPipelineSend struct {
	job ioPipelineJob
	rr  *protocol.PendingRequest
	err error
}

// runIOPipeline sends at most IOPipelineDepth requests ahead of the ordered
// response collector. The sender is the only goroutine; the caller releases a
// bounded outstanding-request token after each response.
func (fs *Share) runIOPipeline(ctx context.Context, jobs int, next func() (ioPipelineJob, bool), handle func(context.Context, ioPipelineJob, *protocol.Response, error) error) error {
	pipeCtx, stop := context.WithCancel(ctx)
	defer stop()
	depth := min(fs.ioPipelineDepth(), uint(jobs))
	tokens := make(chan struct{}, depth)
	sends := make(chan ioPipelineSend, depth)
	go func() {
		defer close(sends)
		for {
			job, ok := next()
			if !ok {
				return
			}
			select {
			case tokens <- struct{}{}:
			case <-pipeCtx.Done():
				return
			}
			pending, err := fs.Request().Append(job.req).Send(pipeCtx)
			if err != nil {
				<-tokens
				sends <- ioPipelineSend{job: job, err: err}
				return
			}
			// A successful send is always published before the sender observes
			// cancellation, so the caller can drain its outstanding request.
			sends <- ioPipelineSend{job: job, rr: pending}
		}
	}()

	var firstErr error
	stopPipeline := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
		stop()
	}
	for sent := range sends {
		if sent.err != nil {
			stopPipeline(sent.err)
			break
		}
		rp, recvErr := sent.rr.Receive()
		err := handle(pipeCtx, sent.job, rp, recvErr)
		if err != nil {
			stopPipeline(err)
			<-tokens
			break
		}
		<-tokens
	}

	// Cancellation may leave successful sends in the channel. Drain every one
	// before returning, including direct READs and caller-owned WRITE buffers.
	for sent := range sends {
		if sent.rr == nil {
			continue
		}
		rp, _ := sent.rr.Receive()
		if rp != nil {
			rp.Close()
		}
		<-tokens
	}
	if firstErr == nil && ctx.Err() != nil {
		firstErr = ctx.Err()
	}
	return firstErr
}

func (fs *Share) makeReadRequest(fd wire.FileId, b []byte, job ioPipelineJob) wire.Packet {
	remaining := job.end - job.start
	buf := b[job.start:job.end]
	req := &wire.ReadRequest{
		Padding:         0,
		Flags:           0,
		Length:          uint32(remaining),
		Offset:          uint64(job.off),
		MinimumCount:    1,
		Channel:         0,
		RemainingBytes:  0,
		ReadChannelInfo: nil,
		FileId:          fd,
	}
	if remaining >= clientMinBufSize {
		// Bound the direct-receive buffer to the requested Length so a server
		// cannot copy more than Length bytes into b. [MS-SMB2] 3.3.5.12
		// requires the response DataLength to be capped at the requested Length.
		return &protocol.DirectReadRequest{ReadRequest: req, Buffer: buf}
	}
	return req
}

// readAt fills the requested range concurrently by fixed, non-overlapping
// chunks. A short successful response is retried only inside its assigned
// chunk, so out-of-order responses cannot overlap a neighboring range.
func (fs *Share) readAt(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	maxChunk := fs.maxReadSize(0)
	if maxChunk <= 0 {
		return 0, errors.New("smb2: invalid maximum read size")
	}
	if fs.ioPipelineDepth() == 1 || (fs.treeConn.ShareType() != 0 && fs.treeConn.ShareType() != wire.SMB2_SHARE_TYPE_DISK) || len(b) <= maxChunk {
		return fs.readAtSequential(ctx, fd, b, off)
	}

	start := 0
	next := func() (ioPipelineJob, bool) {
		if start >= len(b) {
			return ioPipelineJob{}, false
		}
		end := start + min(maxChunk, len(b)-start)
		job := ioPipelineJob{start: start, end: end, off: off + int64(start)}
		job.req = fs.makeReadRequest(fd, b, job)
		start = end
		return job, true
	}
	handle := func(pipeCtx context.Context, job ioPipelineJob, rp *protocol.Response, recvErr error) error {
		readN, readErr := fs.parseReadResponse(b, job, rp, recvErr)
		n += readN
		if rp != nil {
			rp.Close()
		}
		if readErr == nil && readN < job.end-job.start {
			more, moreErr := fs.readAtSequential(pipeCtx, fd,
				b[job.start+readN:job.end],
				job.off+int64(readN))
			n += more
			return moreErr
		}
		if errors.Is(readErr, erref.STATUS_BUFFER_OVERFLOW) && readN > 0 {
			more, moreErr := fs.readAtSequential(pipeCtx, fd,
				b[job.start+readN:job.end],
				job.off+int64(readN))
			n += more
			if moreErr != nil {
				return moreErr
			}
			return nil
		}
		if errors.Is(readErr, erref.STATUS_END_OF_FILE) {
			return io.EOF
		}
		return readErr
	}
	err = fs.runIOPipeline(ctx, 1+(len(b)-1)/maxChunk, next, handle)
	return n, err
}

func (fs *Share) readAtSequential(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	for n < len(b) {
		readN, readErr := fs.readAtChunk(ctx, fd, b[n:], off+int64(n))
		n += readN
		if readErr != nil {
			if errors.Is(readErr, erref.STATUS_END_OF_FILE) {
				return n, io.EOF
			}
			if errors.Is(readErr, erref.STATUS_BUFFER_OVERFLOW) && readN > 0 {
				continue
			}
			return n, readErr
		}
	}
	return n, nil
}

func (fs *Share) parseReadResponse(b []byte, job ioPipelineJob, rp *protocol.Response, recvErr error) (int, error) {
	if recvErr != nil {
		if data, ok := protocol.BufferOverflowData(recvErr); ok {
			copy(b[job.start:], data)
			return len(data), recvErr
		}
		return 0, recvErr
	}
	if ext := rp.DirectData(0); ext != nil {
		return len(ext), nil
	}
	r, err := rp.Read(0)
	if err != nil {
		return 0, err
	}
	data := r.Data()
	copy(b[job.start:], data)
	return len(data), nil
}

func (fs *Share) read(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	readN, err := fs.readAtChunk(ctx, fd, b, off)
	if err != nil {
		if status, ok := errors.AsType[erref.NtStatus](err); ok {
			switch status {
			case erref.STATUS_END_OF_FILE:
				return 0, io.EOF
			case erref.STATUS_BUFFER_OVERFLOW:
				if readN > 0 {
					return readN, nil
				}
			}
		}
		return 0, err
	}
	return readN, nil
}

func (fs *Share) writeAt(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}
	maxChunk := fs.maxWriteSize(0)
	if maxChunk <= 0 {
		return 0, errors.New("smb2: invalid maximum write size")
	}
	if fs.ioPipelineDepth() == 1 || (fs.treeConn.ShareType() != 0 && fs.treeConn.ShareType() != wire.SMB2_SHARE_TYPE_DISK) || len(b) <= maxChunk {
		return fs.writeAtSequential(ctx, fd, b, off)
	}
	// Requests already sent for later offsets may complete after an earlier
	// request fails. They are drained before returning, while n reports only
	// the contiguous prefix through the first failed offset.
	start := 0
	next := func() (ioPipelineJob, bool) {
		if start >= len(b) {
			return ioPipelineJob{}, false
		}
		end := start + min(maxChunk, len(b)-start)
		job := ioPipelineJob{
			start: start,
			end:   end,
			off:   off + int64(start),
			req: &wire.WriteRequest{
				Flags:            0,
				Channel:          0,
				RemainingBytes:   0,
				Offset:           uint64(off + int64(start)),
				WriteChannelInfo: nil,
				Data:             b[start:end],
				FileId:           fd,
			},
		}
		start = end
		return job, true
	}
	handle := func(_ context.Context, job ioPipelineJob, rp *protocol.Response, recvErr error) error {
		if recvErr != nil {
			return recvErr
		}
		defer rp.Close()
		count, err := parseWriteResponse(rp, job.end-job.start)
		n += count
		return err
	}
	err = fs.runIOPipeline(ctx, 1+(len(b)-1)/maxChunk, next, handle)
	return n, err
}

func (fs *Share) writeAtSequential(ctx context.Context, fd wire.FileId, b []byte, off int64) (n int, err error) {
	for n < len(b) {
		written, writeErr := fs.writeAtChunk(ctx, fd, b[n:], off+int64(n))
		n += written
		if writeErr != nil {
			return n, writeErr
		}
	}
	return n, nil
}

// MkdirAll mimics os.MkdirAll
func (fs *Share) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	var err error
	path, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(path))
	if err != nil {
		return err
	}

	// Fast path: an existing directory needs no work. Let the server reject
	// creation over an existing file.
	dir, err := fs.Stat(ctx, path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return fs.Mkdir(ctx, path, perm)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && pathpkg.IsSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !pathpkg.IsSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		err = fs.MkdirAll(ctx, path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = fs.Mkdir(ctx, path, perm)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := fs.Lstat(ctx, path)
		if errors.Is(err1, context.Canceled) || errors.Is(err1, context.DeadlineExceeded) {
			return err1
		}
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

var errRemoveTargetNotTraversable = errors.New("smb2: remove target is not a traversable directory")

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error
// it encounters. If the path does not exist, RemoveAll
// returns nil (no error). Symbolic links in parent path components are
// followed, but links at path or within its subtree are removed themselves.
func (fs *Share) RemoveAll(ctx context.Context, path string) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	// An empty path is a no-op, matching os.RemoveAll. A path that only
	// normalizes to empty (".", ".\") names the share root per
	// [MS-SMB2] 2.2.13 and is rejected instead of deleting the root.
	if len(path) == 0 {
		return nil
	}

	var err error
	path, err = pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(path))
	if err != nil {
		return err
	}
	if len(path) == 0 {
		return os.ErrInvalid
	}

	// Simple case: if direct remove works, we're done.
	err = fs.Remove(ctx, path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}

	// SMB CREATE returns attributes as well as a handle, so combine the
	// upstream Lstat and Open without a separate metadata round trip.
	fd, serr := fs.openDirForRemove(ctx, path)
	if serr != nil {
		if errors.Is(serr, errRemoveTargetNotTraversable) {
			return err
		}
		if errors.Is(serr, os.ErrNotExist) || errors.Is(serr, erref.STATUS_NOT_A_DIRECTORY) {
			return nil
		}
		return serr
	}
	path = fd.name

	// Remove contents & return first error, following os/removeall_noat.go.
	err = nil
	for {
		const reqSize = 1024
		var names []string
		var readErr error
		for {
			numErr := 0
			names, readErr = fd.Readdirnames(ctx, reqSize)
			for _, name := range names {
				err1 := fs.RemoveAll(ctx, pathpkg.Join(path, name))
				if err == nil {
					err = err1
				}
				if err1 != nil {
					numErr++
				}
			}
			if numErr != reqSize {
				break
			}
		}

		// Deletion can reshuffle directory entries. Reopen after a batch
		// rather than continuing from a cursor that could skip entries.
		if err == nil && readErr != nil && !errors.Is(readErr, io.EOF) {
			err = readErr
		}
		closeErr := fd.closeAfterOperation(ctx)
		if err == nil {
			err = closeErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if len(names) == 0 {
			break
		}
		if len(names) < reqSize {
			err1 := fs.Remove(ctx, path)
			if err1 == nil || errors.Is(err1, os.ErrNotExist) {
				return err
			}
			if err != nil {
				return err
			}
		}

		fd, serr = fs.openDirForRemove(ctx, path)
		if serr != nil {
			if errors.Is(serr, errRemoveTargetNotTraversable) {
				if err != nil {
					return err
				}
				removeErr := fs.Remove(ctx, path)
				if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
					return nil
				}
				return removeErr
			}
			if errors.Is(serr, os.ErrNotExist) {
				return err
			}
			if err != nil {
				return err
			}
			return serr
		}
		path = fd.name
	}

	// Remove already retries read-only targets, regardless of the client OS.
	err1 := fs.Remove(ctx, path)
	if err1 == nil || errors.Is(err1, os.ErrNotExist) {
		return err
	}
	if err == nil {
		err = err1
	}
	return err
}

func (fs *Share) openDirForRemove(ctx context.Context, name string) (*File, error) {
	if !pathpkg.ValidRelPath(name) {
		return nil, os.ErrInvalid
	}

	// Resolve parent links and open the final component itself. Create uses
	// read/write sharing without delete sharing, keeping the directory pinned.
	res, err := fs.Request().WithFollowSymlinks(true).
		Create(name, wire.FILE_LIST_DIRECTORY|wire.FILE_READ_ATTRIBUTES|wire.READ_CONTROL|wire.SYNCHRONIZE,
			wire.FILE_OPEN, wire.FILE_OPEN_REPARSE_POINT, wire.FILE_ATTRIBUTE_NORMAL).
		Do(ctx)
	if err != nil {
		if errors.Is(err, erref.STATUS_STOPPED_ON_SYMLINK) {
			return nil, errRemoveTargetNotTraversable
		}
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	defer res.Close()

	r, err := res.Create(0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	if r.FileAttributes()&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
		defer cancel()
		_ = fs.closeFile(closeCtx, r.FileId().Decode())
		return nil, errRemoveTargetNotTraversable
	}
	if r.FileAttributes()&wire.FILE_ATTRIBUTE_DIRECTORY == 0 {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
		defer cancel()
		_ = fs.closeFile(closeCtx, r.FileId().Decode())
		return nil, errRemoveTargetNotTraversable
	}

	f := fs.newFile(r, res.ResolvedPath())
	return f, nil
}

func (fs *Share) ioctl(ctx context.Context, fd wire.FileId, req *wire.IoctlRequest) (output []byte, err error) {
	req.FileId = fd

	res, err := fs.Request().Append(req).Do(ctx)
	if err != nil {
		if data, ok := protocol.BufferOverflowData(err); ok {
			return data, err
		}
		return nil, err
	}
	defer res.Close()

	r, err := res.Ioctl(0)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), r.Output()...), nil
}

// Request starts a low-level SMB2 request on this share.
// This API has no compatibility guarantee and may change or be removed.
func (fs *Share) Request() *protocol.Request {
	if fs == nil {
		return (*protocol.Tree)(nil).Request()
	}
	return fs.treeConn.Request()
}

type rawEncoder []byte

func (b rawEncoder) Size() int { return len(b) }

func (b rawEncoder) Encode(p []byte) { copy(p, b) }

const securityInformationComponents = security.Owner |
	security.Group |
	security.DACL |
	security.SACL

func validateSecurityQuery(selection security.Information) error {
	if selection == 0 || selection&^securityInformationComponents != 0 {
		return os.ErrInvalid
	}
	return nil
}

// GetSecurityDescriptor returns the selected owner, group, DACL, and/or SACL
// from the object's Windows security descriptor at the specified path.
// Unrequested fields are nil. A requested absent or NULL ACL is returned as
// security.NullACL, while a regular ACL with no ACEs is empty.
// SACL queries additionally require ACCESS_SYSTEM_SECURITY and the server-side privilege.
func (fs *Share) GetSecurityDescriptor(ctx context.Context, name string, selection security.Information) (*security.Descriptor, error) {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return nil, os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return nil, err
	}
	if err := validateSecurityQuery(selection); err != nil {
		return nil, err
	}

	var access uint32
	if selection&(security.Owner|security.Group|security.DACL) != 0 {
		access |= wire.READ_CONTROL
	}
	if selection&security.SACL != 0 {
		access |= wire.ACCESS_SYSTEM_SECURITY
	}

	req := fs.Request().WithFollowSymlinks(true).
		Create(name, access, wire.FILE_OPEN, 0, wire.FILE_ATTRIBUTE_NORMAL).
		QueryInfo(wire.SMB2_0_INFO_SECURITY, 0, uint32(selection), maxSingleCreditPayloadSize).
		Close()

	res, err := req.Do(ctx)
	if err != nil {
		// [MS-SMB2] 3.3.5.20: a server SHOULD reject a QUERY_INFO whose
		// OutputBufferLength exceeds Connection.MaxTransactSize with
		// STATUS_INVALID_PARAMETER. Do not retry with a length this connection
		// cannot send; keep the original server error instead.
		if required, ok := protocol.RequiredBufferLength(err, 1); ok &&
			required > maxSingleCreditPayloadSize &&
			required <= fs.maxTransactSize(2) {
			req.Get(1).(*wire.QueryInfoRequest).OutputBufferLength = uint32(required)
			res, err = req.Do(ctx)
		}
		if err != nil {
			return nil, &os.PathError{Op: "getsecuritydescriptor", Path: name, Err: err}
		}
	}
	defer res.Close()

	queryRes, err := res.QueryInfo(1)
	if err != nil {
		return nil, &os.PathError{Op: "getsecuritydescriptor", Path: name, Err: err}
	}
	sd, err := queryRes.SecurityDescriptor()
	if err != nil {
		return nil, &os.PathError{Op: "getsecuritydescriptor", Path: name, Err: err}
	}
	return sd, nil
}

// SetSecurityDescriptor applies each non-nil owner, group, DACL, and/or SACL
// to the object's Windows security descriptor at the specified path. Nil fields
// are left unchanged.
// Setting DACL requires WRITE_DAC, owner/group requires WRITE_OWNER, and
// SACL requires ACCESS_SYSTEM_SECURITY plus server privilege.
func (fs *Share) SetSecurityDescriptor(ctx context.Context, name string, descriptor *security.Descriptor) error {
	if ctx == nil {
		panic("nil context")
	}
	if fs == nil {
		return os.ErrInvalid
	}
	name, err := pathpkg.NormalizeRelPath(pathpkg.ToSMBPath(name))
	if err != nil {
		return err
	}
	if descriptor == nil {
		return os.ErrInvalid
	}
	input, err := descriptor.Encode()
	if err != nil {
		return &os.PathError{Op: "setsecuritydescriptor", Path: name, Err: err}
	}
	selection := descriptor.Information()
	if selection == 0 || len(input) == 0 || len(input) > fs.maxTransactSize(2) {
		return os.ErrInvalid
	}

	var access uint32
	if selection&security.DACL != 0 {
		access |= wire.WRITE_DAC
	}
	if selection&(security.Owner|security.Group) != 0 {
		access |= wire.WRITE_OWNER
	}
	if selection&security.SACL != 0 {
		access |= wire.ACCESS_SYSTEM_SECURITY
	}

	res, err := fs.Request().WithFollowSymlinks(true).
		Create(name, access, wire.FILE_OPEN, 0, wire.FILE_ATTRIBUTE_NORMAL).
		SetInfo(wire.SMB2_0_INFO_SECURITY, 0, uint32(selection), rawEncoder(input)).
		Close().
		Do(ctx)
	if err != nil {
		return &os.PathError{Op: "setsecuritydescriptor", Path: name, Err: err}
	}
	res.Close()
	return nil
}
