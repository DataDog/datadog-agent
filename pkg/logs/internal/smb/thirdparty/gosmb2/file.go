package smb2

import (
	"cmp"
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"math"
	"os"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/erref"
	pathpkg "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/path"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/notify"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/protocol"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

// FileDescriptor is the server's identifier for an open file on a share.
type FileDescriptor = wire.FileId

// File is an open file on a share. A File must not be copied.
//
// Concurrent ReadAt calls are supported. WriteAt calls may run concurrently
// with ReadAt or WriteAt on non-overlapping byte ranges; results and ordering
// are unspecified when a write overlaps another operation. Callers must
// serialize all other operations on this File, including Close, against other
// operations. Copies require exclusive use of both files. Context adapters
// share these restrictions. Operations on separate File objects may run
// concurrently.
type File struct {
	fs          *Share
	fd          wire.FileId
	name        string
	isDir       bool
	dirents     []os.FileInfo
	noMoreFiles bool
	fileId      uint64
	volumeId    uint64
	hasIdentity bool

	// readAccess reports whether the open granted read data access, which
	// selects FSCTL_SRV_COPYCHUNK over FSCTL_SRV_COPYCHUNK_WRITE as the copy
	// destination ([MS-SMB2] 2.2.31, 3.3.5.15.6).
	readAccess bool
	appendMode bool

	offset int64

	closed atomic.Bool
}

func (f *File) checkValid(op string) error {
	if f == nil {
		return os.ErrInvalid
	}
	if f.fs == nil || f.closed.Load() {
		return &os.PathError{Op: op, Path: f.name, Err: os.ErrClosed}
	}
	return nil
}

func (f *File) Close(ctx context.Context) error {
	if ctx == nil {
		panic("nil context")
	}
	if f == nil {
		return os.ErrInvalid
	}
	if f.fs == nil || !f.closed.CompareAndSwap(false, true) {
		return &os.PathError{Op: "close", Path: f.name, Err: os.ErrClosed}
	}

	err := f.fs.closeFile(ctx, f.fd)
	if err != nil {
		if errors.Is(err, os.ErrClosed) {
			runtime.SetFinalizer(f, nil)
		} else {
			f.closed.Store(false)
		}
		return &os.PathError{Op: "close", Path: f.name, Err: err}
	}
	runtime.SetFinalizer(f, nil)
	return nil
}

func (f *File) closeAfterOperation(ctx context.Context) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clientCleanupTimeout)
	defer cancel()
	return f.Close(closeCtx)
}

func (f *File) Sync(ctx context.Context) (err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("sync"); err != nil {
		return err
	}
	if err := f.fs.flush(ctx, f.fd); err != nil {
		return &os.PathError{Op: "sync", Path: f.name, Err: err}
	}
	return nil
}

func (f *File) Name() string {
	if f == nil {
		return ""
	}
	return f.name
}

// Fd returns a copy of the server's descriptor for the open file. It returns
// the zero value for a nil File. Closing the file does not clear the descriptor.
func (f *File) Fd() FileDescriptor {
	if f == nil {
		return FileDescriptor{}
	}
	return f.fd
}

func (f *File) Truncate(ctx context.Context, size int64) error {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("truncate"); err != nil {
		return err
	}
	if err := f.fs.truncate(ctx, &f.fd, f.name, size); err != nil {
		return &os.PathError{Op: "truncate", Path: f.name, Err: err}
	}
	return nil
}

func (f *File) Chmod(ctx context.Context, mode os.FileMode) error {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("chmod"); err != nil {
		return err
	}
	if err := f.fs.chmod(ctx, &f.fd, f.name, mode, true); err != nil {
		return &os.PathError{Op: "chmod", Path: f.name, Err: err}
	}
	return nil
}

func (f *File) Read(ctx context.Context, b []byte) (n int, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("read"); err != nil {
		return 0, err
	}
	if !validFileRange(f.offset, len(b)) {
		return 0, os.ErrInvalid
	}

	// Reads a single chunk of at most maxReadSize bytes. If b is larger, the
	// read returns short and the caller must retry to fetch the remainder.
	n, err = f.fs.read(ctx, f.fd, b, f.offset)
	f.offset += int64(n)
	if err != nil {
		if err == io.EOF || errors.Is(err, erref.STATUS_END_OF_FILE) {
			return n, io.EOF
		}
		return n, &os.PathError{Op: "read", Path: f.name, Err: err}
	}
	return n, nil
}

// ReadAt implements io.ReaderAt.
func (f *File) ReadAt(ctx context.Context, b []byte, off int64) (n int, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("read"); err != nil {
		return 0, err
	}
	if !validFileRange(off, len(b)) {
		return 0, os.ErrInvalid
	}
	n, err = f.fs.readAt(ctx, f.fd, b, off)
	if err == nil && n < len(b) {
		return n, io.EOF
	}
	if err != nil {
		if err == io.EOF || errors.Is(err, erref.STATUS_END_OF_FILE) {
			return n, io.EOF
		}
		return n, &os.PathError{Op: "read", Path: f.name, Err: err}
	}
	return n, nil
}

// Write writes b at the current offset. If a pipelined write fails, requests
// for later offsets may already have modified the file even though n reports
// only the contiguous prefix through the failed offset.
func (f *File) Write(ctx context.Context, b []byte) (n int, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("write"); err != nil {
		return 0, err
	}
	if f.appendMode && len(b) > 0 {
		end, err := f.endOfFile(ctx)
		if err != nil {
			return 0, &os.PathError{Op: "write", Path: f.name, Err: err}
		}
		f.offset = end
	}
	if !validFileRange(f.offset, len(b)) {
		return 0, os.ErrInvalid
	}

	n, err = f.fs.writeAt(ctx, f.fd, b, f.offset)
	if n > 0 {
		f.offset += int64(n)
	}
	if err != nil {
		if n < 0 {
			n = 0
		}
		return n, &os.PathError{Op: "write", Path: f.name, Err: err}
	}

	return n, nil
}

// WriteAt implements io.WriterAt. It rejects files opened with O_APPEND.
// If a pipelined write fails, requests for later offsets may already have
// modified the file even though n reports only the contiguous prefix through
// the failed offset.
func (f *File) WriteAt(ctx context.Context, b []byte, off int64) (n int, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("write"); err != nil {
		return 0, err
	}
	if f.appendMode {
		return 0, errors.New("smb2: invalid use of WriteAt on file opened with O_APPEND")
	}
	if !validFileRange(off, len(b)) {
		return 0, os.ErrInvalid
	}
	n, err = f.fs.writeAt(ctx, f.fd, b, off)
	if err != nil {
		if n < 0 {
			n = 0
		}
		return n, &os.PathError{Op: "write", Path: f.name, Err: err}
	}
	return n, nil
}

// Seek implements io.Seeker.
func (f *File) Seek(ctx context.Context, offset int64, whence int) (ret int64, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("seek"); err != nil {
		return 0, err
	}

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		end, err := f.endOfFile(ctx)
		if err != nil {
			return 0, &os.PathError{Op: "seek", Path: f.name, Err: err}
		}
		newOffset = offset + end
	default:
		return 0, os.ErrInvalid
	}

	if newOffset < 0 {
		return 0, os.ErrInvalid
	}

	if f.isDir && whence == io.SeekStart && offset == 0 {
		// A failed restart may still move the server cursor. Discard the old
		// scan before sending so later reads cannot mix the two scans.
		f.dirents = nil
		f.noMoreFiles = false
		entries, err := f.fs.readdir(ctx, f.fd, "*", true)
		ended := errors.Is(err, erref.STATUS_NO_MORE_FILES) || errors.Is(err, erref.STATUS_NO_SUCH_FILE)
		if err != nil && !ended {
			return 0, &os.PathError{Op: "seek", Path: f.name, Err: err}
		}
		f.dirents = entries
		f.noMoreFiles = ended || len(entries) == 0
	}

	f.offset = newOffset
	return f.offset, nil
}

func (f *File) endOfFile(ctx context.Context) (int64, error) {
	res, err := f.fs.Request().WithFollowSymlinks(true).WithFileID(f.fd).
		QueryInfo(wire.SMB2_0_INFO_FILE, wire.FileStandardInformation, 0, 24).
		Do(ctx)
	if err != nil {
		return 0, err
	}
	defer res.Close()

	queryRes, err := res.QueryInfo(0)
	if err != nil {
		return 0, err
	}
	info, err := queryRes.FileStandardInformation()
	if err != nil {
		return 0, err
	}
	return info.EndOfFile(), nil
}

func computeChmodAttrs(attrs uint32, mode os.FileMode) uint32 {
	if attrs&wire.FILE_ATTRIBUTE_DIRECTORY == 0 {
		attrs |= wire.FILE_ATTRIBUTE_NORMAL
	}

	if mode&0o200 != 0 {
		attrs &^= wire.FILE_ATTRIBUTE_READONLY
	} else {
		attrs |= wire.FILE_ATTRIBUTE_READONLY
	}
	return attrs
}

type FileStat struct {
	CreationTime   time.Time
	LastAccessTime time.Time
	LastWriteTime  time.Time
	ChangeTime     time.Time
	EndOfFile      int64
	AllocationSize int64
	FileAttributes uint32
	ReparseTag     uint32
	FileName       string

	// FileId is the server's 64-bit file identifier (QFid DiskFileId or the
	// directory entry's FileId), not an SMB handle. Zero and all-ones values
	// cannot identify a file. Identifiers may be reused after deletion.
	FileId uint64
	// VolumeId is the QFid volume identifier. It is not populated by ReadDir
	// or Readdir, nor when the server omits QFid. Zero alone does not indicate
	// whether the identifier was returned.
	VolumeId uint64

	hasIdentity bool
}

// SameFile reports whether fi1 and fi2 have matching server-provided volume
// and file identifiers. The caller must ensure both describe files on the
// same SMB server, including after any DFS or cross-share link resolution;
// SameFile does not verify their origin.
//
// Both values must be *FileStat values with usable QFid identifiers obtained
// by Stat, Lstat, or File.Stat. ReadDir and Readdir results lack volume IDs
// and always compare false, as do nil values and missing or unsupported IDs.
// File identifiers may be reused after deletion; they are not permanent IDs.
func SameFile(fi1, fi2 os.FileInfo) bool {
	a, ok := fi1.(*FileStat)
	if !ok || a == nil {
		return false
	}
	b, ok := fi2.(*FileStat)
	if !ok || b == nil {
		return false
	}
	return a.hasIdentity && b.hasIdentity &&
		a.FileId != 0 && a.FileId != ^uint64(0) &&
		a.FileId == b.FileId && a.VolumeId == b.VolumeId
}

func (fs *FileStat) Name() string {
	return fs.FileName
}

func (fs *FileStat) Size() int64 {
	return fs.EndOfFile
}

func (fs *FileStat) Mode() os.FileMode {
	var m os.FileMode

	// Name-surrogate reparse points must not be traversed as directories.
	nameSurrogate := fs.FileAttributes&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 && fs.ReparseTag&0x20000000 != 0
	if !nameSurrogate && fs.FileAttributes&wire.FILE_ATTRIBUTE_DIRECTORY != 0 {
		m |= os.ModeDir | 0o111
	}

	if fs.FileAttributes&wire.FILE_ATTRIBUTE_READONLY != 0 {
		m |= 0o444
	} else {
		m |= 0o666
	}

	if fs.FileAttributes&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		switch fs.ReparseTag {
		case wire.IO_REPARSE_TAG_SYMLINK:
			m |= os.ModeSymlink
		case wire.IO_REPARSE_TAG_AF_UNIX:
			m |= os.ModeSocket
		case wire.IO_REPARSE_TAG_DEDUP:
			// Like os on Windows, treat deduplicated files as ordinary files.
		default:
			m |= os.ModeIrregular
		}
	}

	return m
}

func (fs *FileStat) ModTime() time.Time {
	return fs.LastWriteTime
}

func (fs *FileStat) IsDir() bool {
	return fs.Mode().IsDir()
}

func (fs *FileStat) Sys() any {
	return fs
}

func newFileStat(creation, access, write, change time.Time, size, allocSize int64, attrs uint32, name string) *FileStat {
	return &FileStat{
		CreationTime:   creation,
		LastAccessTime: access,
		LastWriteTime:  write,
		ChangeTime:     change,
		EndOfFile:      size,
		AllocationSize: allocSize,
		FileAttributes: attrs,
		FileName:       name,
	}
}

func newFileStatFromCreateResponse(r wire.CreateResponseDecoder, name string) *FileStat {
	stat := newFileStat(
		r.CreationTime().Time(),
		r.LastAccessTime().Time(),
		r.LastWriteTime().Time(),
		r.ChangeTime().Time(),
		r.EndofFile(),
		r.AllocationSize(),
		r.FileAttributes(),
		pathpkg.Base(name),
	)
	if identity := r.QueryOnDiskID(); identity != nil {
		stat.FileId = identity.DiskFileId()
		stat.VolumeId = identity.VolumeId()
		stat.hasIdentity = true
	}
	return stat
}

func newFileStatFromFileNetworkOpenInformation(info wire.FileNetworkOpenInformationDecoder, name string) *FileStat {
	return newFileStat(
		info.CreationTime().Time(),
		info.LastAccessTime().Time(),
		info.LastWriteTime().Time(),
		info.ChangeTime().Time(),
		info.EndOfFile(),
		info.AllocationSize(),
		info.FileAttributes(),
		pathpkg.Base(name),
	)
}

func newFileStatFromFileIdBothDirectoryInformation(info wire.FileIdBothDirectoryInformationDecoder, name string) *FileStat {
	stat := newFileStat(
		info.CreationTime().Time(),
		info.LastAccessTime().Time(),
		info.LastWriteTime().Time(),
		info.ChangeTime().Time(),
		info.EndOfFile(),
		info.AllocationSize(),
		info.FileAttributes(),
		name,
	)
	stat.FileId = info.FileId()
	if stat.FileAttributes&wire.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		stat.ReparseTag = info.EaSize()
	}
	return stat
}

func (f *File) Stat(ctx context.Context) (os.FileInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("stat"); err != nil {
		return nil, err
	}
	fi, err := f.fs.stat(ctx, &f.fd, f.name)
	if err != nil {
		return nil, &os.PathError{Op: "stat", Path: f.name, Err: err}
	}
	stat := fi.(*FileStat)
	stat.FileId = f.fileId
	stat.VolumeId = f.volumeId
	stat.hasIdentity = f.hasIdentity

	return fi, nil
}

func (f *File) Statfs(ctx context.Context) (FileFsInfo, error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("statfs"); err != nil {
		return nil, err
	}
	fi, err := f.fs.statfs(ctx, &f.fd, f.name)
	if err != nil {
		return nil, &os.PathError{Op: "statfs", Path: f.name, Err: err}
	}
	return fi, nil
}

type FileFsInfo interface {
	BlockSize() uint64
	FragmentSize() uint64
	TotalBlockCount() uint64
	FreeBlockCount() uint64
	AvailableBlockCount() uint64
}

type fileFsFullSizeInformation struct {
	TotalAllocationUnits           int64
	CallerAvailableAllocationUnits int64
	ActualAvailableAllocationUnits int64
	SectorsPerAllocationUnit       uint32
	BytesPerSector                 uint32
}

func (fi *fileFsFullSizeInformation) BlockSize() uint64 {
	return uint64(fi.SectorsPerAllocationUnit) * uint64(fi.BytesPerSector)
}

func (fi *fileFsFullSizeInformation) FragmentSize() uint64 {
	return uint64(fi.SectorsPerAllocationUnit)
}

func (fi *fileFsFullSizeInformation) TotalBlockCount() uint64 {
	return uint64(fi.TotalAllocationUnits)
}

func (fi *fileFsFullSizeInformation) FreeBlockCount() uint64 {
	return uint64(fi.ActualAvailableAllocationUnits)
}

func (fi *fileFsFullSizeInformation) AvailableBlockCount() uint64 {
	return uint64(fi.CallerAvailableAllocationUnits)
}

func parseFsFullSizeInfo(r1 *protocol.QueryInfoResponse) (FileFsInfo, error) {
	info, err := r1.FileFsFullSizeInformation()
	if err != nil {
		return nil, err
	}

	return &fileFsFullSizeInformation{
		TotalAllocationUnits:           info.TotalAllocationUnits(),
		CallerAvailableAllocationUnits: info.CallerAvailableAllocationUnits(),
		ActualAvailableAllocationUnits: info.ActualAvailableAllocationUnits(),
		SectorsPerAllocationUnit:       info.SectorsPerAllocationUnit(),
		BytesPerSector:                 info.BytesPerSector(),
	}, nil
}

func (f *File) Readdir(ctx context.Context, n int) (fi []os.FileInfo, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("readdir"); err != nil {
		return nil, err
	}

	if !f.noMoreFiles {
		if f.dirents == nil {
			f.dirents = []os.FileInfo{}
		}
		for n <= 0 || n > len(f.dirents) {
			dirents, err := f.fs.readdir(ctx, f.fd, "*", false)
			if len(dirents) > 0 {
				f.dirents = append(f.dirents, dirents...)
			}
			if err != nil {
				// Some servers (e.g. Samba) report STATUS_NO_SUCH_FILE on the
				// first QUERY_DIRECTORY of an empty directory instead of
				// STATUS_NO_MORE_FILES ([MS-FSA] 2.1.5.6.3). Treat it as a
				// normal end-of-directory.
				if errors.Is(err, erref.STATUS_NO_MORE_FILES) || errors.Is(err, erref.STATUS_NO_SUCH_FILE) {
					f.noMoreFiles = true
					break
				}
				fi = f.dirents
				f.dirents = []os.FileInfo{}
				return fi, &os.PathError{Op: "readdir", Path: f.name, Err: err}
			}
			if len(dirents) == 0 {
				f.noMoreFiles = true
				break
			}
		}
	}

	fi = f.dirents

	if n > 0 {
		if len(fi) == 0 {
			return fi, io.EOF
		}

		if len(fi) < n {
			f.dirents = []os.FileInfo{}
			return fi, nil
		}

		f.dirents = fi[n:]
		return fi[:n:n], nil
	}

	f.dirents = []os.FileInfo{}

	return fi, nil
}

func (f *File) ReadDir(ctx context.Context, n int) (dirents []iofs.DirEntry, err error) {
	if ctx == nil {
		panic("nil context")
	}
	infos, err := f.Readdir(ctx, n)
	if err != nil && len(infos) == 0 {
		return nil, err
	}
	dirents = make([]iofs.DirEntry, len(infos))
	for i, info := range infos {
		dirents[i] = iofs.FileInfoToDirEntry(info)
	}
	return dirents, err
}

func (f *File) Readdirnames(ctx context.Context, n int) (names []string, err error) {
	if ctx == nil {
		panic("nil context")
	}
	fi, err := f.Readdir(ctx, n)
	if err != nil && len(fi) == 0 {
		return nil, err
	}

	names = make([]string, len(fi))

	for i, st := range fi {
		names[i] = st.Name()
	}

	return names, err
}

func (f *File) readdirAll(ctx context.Context, queryRes *protocol.QueryDirectoryResponse) ([]os.FileInfo, error) {
	entries, err := queryRes.FileIdBothDirectoryInformation()
	if err != nil {
		return nil, err
	}
	fis := parseDirectoryEntries(entries)

	f.dirents = fis

	moreFis, err := f.Readdir(ctx, -1)

	slices.SortFunc(moreFis, func(a, b os.FileInfo) int { return cmp.Compare(a.Name(), b.Name()) })

	return moreFis, err
}

func isDotOrDotDot(info wire.FileIdBothDirectoryInformationDecoder) bool {
	return wire.IsDotDirectoryName(info.FileNameBytes())
}

func parseDirectoryEntries(entries []wire.FileIdBothDirectoryInformationDecoder) (fi []os.FileInfo) {
	fi = make([]os.FileInfo, 0, len(entries))
	for _, info := range entries {
		if !isDotOrDotDot(info) {
			fi = append(fi, newFileStatFromFileIdBothDirectoryInformation(info, info.FileName()))
		}
	}
	return fi
}

// WithContext returns an adapter using ctx and sharing this File's state.
func (f *File) WithContext(ctx context.Context) interface {
	iofs.File
	iofs.ReadDirFile
	io.Writer
	io.Seeker
	io.ReaderAt
	io.WriterAt
	io.ReaderFrom
	io.WriterTo
} {
	if ctx == nil {
		panic("nil context")
	}
	if f == nil {
		return nil
	}
	return &boundFile{file: f, ctx: ctx}
}

// ReadFrom implements io.ReadFrom.
// A source bound File on the same share uses server-side copy when both file
// offsets match. Other copies use ordinary reads and writes.
func (f *File) ReadFrom(ctx context.Context, r io.Reader) (n int64, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if r == nil {
		return 0, os.ErrInvalid
	}
	rw, ok := r.(*boundFile)
	var rf *File
	if ok && rw != nil {
		rf = rw.file
	}
	if ok && rf == f {
		return 0, os.ErrInvalid
	}
	if err := f.checkValid("write"); err != nil {
		return 0, err
	}
	if ok && rf != nil && rf.fs != nil && f.fs != nil && rf.fs.treeConn == f.fs.treeConn && !f.appendMode {
		if err := rf.checkValid("read"); err != nil {
			return 0, err
		}
		copyCtx, cancel := joinCopyContext(ctx, rw.ctx)

		supported, n, err := f.fs.copyFile(copyCtx, rf.fd, f.fd, rf.name, f.name, rf.offset, f.offset, f.readAccess)
		cancel()
		if supported {
			if n > 0 {
				rf.offset += n
				f.offset += n
			}
			return n, err
		}

		maxBufferSize := min(f.fs.maxReadSize(0), f.fs.maxWriteSize(0))

		return copyBuffer(r, &contextWriter{ctx: ctx, file: f}, make([]byte, maxBufferSize))
	}

	return copyBuffer(r, &contextWriter{ctx: ctx, file: f}, make([]byte, f.fs.maxWriteSize(0)))
}

// WriteTo implements io.WriteTo.
// A destination bound File on the same share uses server-side copy when both
// file offsets match. Other copies use ordinary reads and writes.
func (f *File) WriteTo(ctx context.Context, w io.Writer) (n int64, err error) {
	if ctx == nil {
		panic("nil context")
	}
	if w == nil {
		return 0, os.ErrInvalid
	}
	ww, ok := w.(*boundFile)
	var wf *File
	if ok && ww != nil {
		wf = ww.file
	}
	if ok && wf == f {
		return 0, os.ErrInvalid
	}
	if err := f.checkValid("read"); err != nil {
		return 0, err
	}
	if ok && wf != nil && wf.fs != nil && f.fs != nil && wf.fs.treeConn == f.fs.treeConn && !wf.appendMode {
		if err := wf.checkValid("write"); err != nil {
			return 0, err
		}
		copyCtx, cancel := joinCopyContext(ctx, ww.ctx)

		supported, n, err := f.fs.copyFile(copyCtx, f.fd, wf.fd, f.name, wf.name, f.offset, wf.offset, wf.readAccess)
		cancel()
		if supported {
			if n > 0 {
				f.offset += n
				wf.offset += n
			}
			return n, err
		}

		maxBufferSize := min(f.fs.maxReadSize(0), f.fs.maxWriteSize(0))

		return copyBuffer(&contextReader{ctx: ctx, file: f}, w, make([]byte, maxBufferSize))
	}

	return copyBuffer(&contextReader{ctx: ctx, file: f}, w, make([]byte, f.fs.maxReadSize(0)))
}

// joinCopyContext scopes peer cancellation to the server-side copy request.
// Cleanup unregisters the callback and releases the derived context.
func joinCopyContext(ctx, peer context.Context) (context.Context, context.CancelFunc) {
	if peer.Done() == nil {
		return ctx, func() {}
	}
	// Pass Err, rather than Cause, so caller-defined cancellation causes do not
	// replace context.Canceled or context.DeadlineExceeded in filesystem errors.
	joined, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, func() { cancel(ctx.Err()) })
	stopPeer := context.AfterFunc(peer, func() { cancel(peer.Err()) })
	if err := ctx.Err(); err != nil {
		cancel(err)
	}
	if err := peer.Err(); err != nil {
		cancel(err)
	}
	deadline, hasDeadline := ctx.Deadline()
	if other, ok := peer.Deadline(); ok && (!hasDeadline || other.Before(deadline)) {
		deadline, hasDeadline = other, true
	}
	return copyContext{Context: joined, deadline: deadline, hasDeadline: hasDeadline}, func() {
		stop()
		stopPeer()
		cancel(context.Canceled)
	}
}

type copyContext struct {
	context.Context
	deadline    time.Time
	hasDeadline bool
}

func (ctx copyContext) Err() error                  { return context.Cause(ctx.Context) }
func (ctx copyContext) Deadline() (time.Time, bool) { return ctx.deadline, ctx.hasDeadline }

func copyBuffer(r io.Reader, w io.Writer, buf []byte) (n int64, err error) {
	for {
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[:nr])
			if nw > 0 {
				n += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return
}

// ByteRange identifies a byte range associated with a File handle. A zero
// Length is sent to the server unchanged; it is not interpreted as EOF.
type ByteRange struct {
	Offset int64
	Length int64
}

// LockRange describes one shared or exclusive byte-range lock.
type LockRange struct {
	Range     ByteRange
	Exclusive bool
}

const maxLockRequestSize = 64 * 1024

func validateByteRange(r ByteRange) error {
	if r.Offset < 0 || r.Length < 0 {
		return os.ErrInvalid
	}
	if r.Length > 0 && r.Length-1 > math.MaxInt64-r.Offset {
		return os.ErrInvalid
	}
	return nil
}

func validateLockRangeCount(count int) error {
	if count == 0 || count > math.MaxUint16 || 64+24+count*24 > maxLockRequestSize {
		return os.ErrInvalid
	}
	return nil
}

// Lock acquires shared or exclusive byte-range locks on this File handle.
// With failImmediately false, multiple ranges are rejected because SMB2 only
// permits a multi-range request to be immediate. A transport failure leaves
// the server-side lock state uncertain; multiple ranges are not retried or
// rolled back.
// Cancellation sends SMB2 CANCEL and waits for the server's final result.
// A successful lock is returned as success even if ctx has expired; cancellation
// does not release locks. The request rules are defined by [MS-SMB2] 3.2.4.19.
func (f *File) Lock(ctx context.Context, ranges []LockRange, failImmediately bool) error {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("lock"); err != nil {
		return err
	}
	if !failImmediately && len(ranges) > 1 {
		return os.ErrInvalid
	}
	if err := validateLockRangeCount(len(ranges)); err != nil {
		return err
	}

	locks := make([]wire.LockElement, len(ranges))
	for i, lock := range ranges {
		if err := validateByteRange(lock.Range); err != nil {
			return err
		}
		flags := uint32(wire.SMB2_LOCKFLAG_SHARED_LOCK)
		if lock.Exclusive {
			flags = wire.SMB2_LOCKFLAG_EXCLUSIVE_LOCK
		}
		if failImmediately {
			flags |= wire.SMB2_LOCKFLAG_FAIL_IMMEDIATELY
		}
		locks[i] = wire.LockElement{
			Offset: uint64(lock.Range.Offset),
			Length: uint64(lock.Range.Length),
			Flags:  flags,
		}
	}

	res, err := f.fs.Request().WithFollowSymlinks(true).WithFileID(f.fd).Lock(locks).Do(ctx)
	if err != nil {
		return &os.PathError{Op: "lock", Path: f.name, Err: err}
	}
	res.Close()
	return nil
}

// Unlock releases byte-range locks on this File handle. Each range must be
// identical to the range used to acquire the lock; the server may process a
// multi-range unlock only partially before returning an error. A transport
// failure leaves the server-side lock state uncertain and is not retried.
// The exact-match and partial-processing rules are from [MS-SMB2] 3.3.5.14.1.
func (f *File) Unlock(ctx context.Context, ranges []ByteRange) error {
	if ctx == nil {
		panic("nil context")
	}
	if err := f.checkValid("unlock"); err != nil {
		return err
	}
	if err := validateLockRangeCount(len(ranges)); err != nil {
		return err
	}

	locks := make([]wire.LockElement, len(ranges))
	for i, r := range ranges {
		if err := validateByteRange(r); err != nil {
			return err
		}
		locks[i] = wire.LockElement{
			Offset: uint64(r.Offset),
			Length: uint64(r.Length),
			Flags:  wire.SMB2_LOCKFLAG_UNLOCK,
		}
	}

	res, err := f.fs.Request().WithFollowSymlinks(true).WithFileID(f.fd).Lock(locks).Do(ctx)
	if err != nil {
		return &os.PathError{Op: "unlock", Path: f.name, Err: err}
	}
	res.Close()
	return nil
}

const changeFilterMask = notify.FileName | notify.DirName |
	notify.Attributes | notify.Size | notify.LastWrite |
	notify.LastAccess | notify.Creation | notify.EA |
	notify.Security | notify.StreamName | notify.StreamSize |
	notify.StreamWrite

// WaitForChange waits for one directory change notification. The server fixes
// the completion filter and watch mode from the first CHANGE_NOTIFY request on
// the open and ignores them in later requests ([MS-SMB2] 3.3.1.3); use another
// Open for a different monitor. A canceled call can consume a notification, and
// the server does not provide a complete change history, so callers must issue
// another call when they want to continue monitoring. Invalid filters return
// os.ErrInvalid. The server validates whether the open supports notifications;
// server errors are returned in an os.PathError with the operation and file path.
func (f *File) WaitForChange(ctx context.Context, filter notify.Filter, recursive bool) (notify.Result, error) {
	if ctx == nil {
		panic("nil context")
	}

	var result notify.Result
	if err := f.checkValid("waitforchange"); err != nil {
		return result, err
	}
	if filter == 0 || filter&^changeFilterMask != 0 {
		return result, os.ErrInvalid
	}

	res, err := f.fs.Request().WithFollowSymlinks(true).WithFileID(f.fd).
		ChangeNotify(uint32(filter), recursive, maxSingleCreditPayloadSize).
		Do(ctx)
	if err != nil {
		return result, &os.PathError{Op: "waitforchange", Path: f.name, Err: err}
	}
	defer res.Close()

	header, err := res.Header(0)
	if err != nil {
		return result, &os.PathError{Op: "waitforchange", Path: f.name, Err: err}
	}
	status := erref.NtStatus(header.Status())
	r, err := res.ChangeNotify(0)
	if err != nil {
		return result, &os.PathError{Op: "waitforchange", Path: f.name, Err: err}
	}
	if status == erref.STATUS_NOTIFY_ENUM_DIR {
		return notify.Result{RescanRequired: true}, nil
	}

	if len(r.Output()) == 0 {
		return notify.Result{RescanRequired: true}, nil
	}

	entries, err := r.FileNotifyInformation()
	if err != nil {
		return result, &os.PathError{Op: "waitforchange", Path: f.name, Err: err}
	}
	events := make([]notify.Event, 0, len(entries))
	for _, e := range entries {
		events = append(events, notify.Event{Action: notify.Action(e.Action()), Name: e.FileName()})
	}
	return notify.Result{Events: events}, nil
}
