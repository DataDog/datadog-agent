package smb2

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/directory"
	pathpkg "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/path"
)

// boundShare binds a context to a Share for use with io/fs. It does not
// duplicate the Share's connection or mutable state.
type boundShare struct {
	share *Share
	ctx   context.Context
	root  string
}

func (s *boundShare) path(name string) string {
	return pathpkg.Join(s.root, pathpkg.Normalize(pathpkg.ToSMBPath(name)))
}

func contextPathError(op, name string, err error) error {
	if err == nil {
		return nil
	}
	if pe, ok := errors.AsType[*os.PathError](err); ok {
		return &os.PathError{Op: op, Path: name, Err: pe.Err}
	}
	return err
}

func (s *boundShare) checkValid() error {
	if s == nil || s.share == nil {
		return os.ErrInvalid
	}
	return nil
}

func (s *boundShare) Open(name string) (iofs.File, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(name) {
		return nil, os.ErrInvalid
	}
	f, err := s.share.Open(s.ctx, s.path(name))
	if err != nil {
		return nil, contextPathError("open", name, err)
	}
	return &boundFile{file: f, ctx: s.ctx}, nil
}

func (s *boundShare) Stat(name string) (iofs.FileInfo, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(name) {
		return nil, os.ErrInvalid
	}
	fi, err := s.share.Stat(s.ctx, s.path(name))
	return fi, contextPathError("stat", name, err)
}

func (s *boundShare) Lstat(name string) (iofs.FileInfo, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(name) {
		return nil, os.ErrInvalid
	}
	fi, err := s.share.Lstat(s.ctx, s.path(name))
	return fi, contextPathError("lstat", name, err)
}

func (s *boundShare) ReadFile(name string) ([]byte, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(name) {
		return nil, os.ErrInvalid
	}
	b, err := s.share.ReadFile(s.ctx, s.path(name))
	return b, contextPathError("readfile", name, err)
}

func (s *boundShare) ReadDir(name string) ([]iofs.DirEntry, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(name) {
		return nil, os.ErrInvalid
	}
	fis, err := s.share.ReadDir(s.ctx, s.path(name))
	if err != nil && len(fis) == 0 {
		return nil, contextPathError("readdir", name, err)
	}
	entries := make([]iofs.DirEntry, len(fis))
	for i, fi := range fis {
		entries[i] = iofs.FileInfoToDirEntry(fi)
	}
	return entries, contextPathError("readdir", name, err)
}

func (s *boundShare) ReadLink(name string) (string, error) {
	if err := s.checkValid(); err != nil {
		return "", err
	}
	if !pathpkg.ValidPosixPath(name) {
		return "", os.ErrInvalid
	}
	target, err := s.share.Readlink(s.ctx, s.path(name))
	if err != nil {
		return "", contextPathError("readlink", name, err)
	}
	return pathpkg.ToPOSIXPath(target), nil
}

func (s *boundShare) Glob(pattern string) ([]string, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	return pathpkg.GlobFS(pattern, s.Lstat, func(dir, pattern string) ([]string, error) {
		reader, err := directory.Open(s.ctx, s.share.Request, s.path(dir))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, &os.PathError{Op: "glob", Path: dir, Err: err}
			}
			// Glob ignores directory lookup failures.
			return nil, nil
		}
		defer reader.Close()
		names, err := reader.Names(s.ctx, pathpkg.SMBSearchPattern(pattern))
		if err != nil {
			if directory.IsGlobIOError(err) {
				return nil, nil
			}
			return nil, &os.PathError{Op: "glob", Path: dir, Err: err}
		}
		return names, nil
	})
}

func (s *boundShare) Sub(dir string) (iofs.FS, error) {
	if err := s.checkValid(); err != nil {
		return nil, err
	}
	if !pathpkg.ValidPosixPath(dir) {
		return nil, os.ErrInvalid
	}
	root := s.path(dir)
	return &boundShare{share: s.share, ctx: s.ctx, root: root}, nil
}

// boundFile binds a context to a core File. All wrappers share the core
// file's offset, directory cursor, and closed state.
type boundFile struct {
	file *File
	ctx  context.Context
}

func (f *boundFile) checkValid() error {
	if f == nil || f.file == nil {
		return os.ErrInvalid
	}
	return nil
}

func (f *boundFile) Close() error {
	if err := f.checkValid(); err != nil {
		return err
	}
	ctx := f.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return f.file.Close(ctx)
}

func (f *boundFile) Stat() (iofs.FileInfo, error) {
	if err := f.checkValid(); err != nil {
		return nil, err
	}
	return f.file.Stat(f.ctx)
}

func (f *boundFile) Read(p []byte) (int, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.Read(f.ctx, p)
}

func (f *boundFile) ReadAt(p []byte, off int64) (int, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.ReadAt(f.ctx, p, off)
}

func (f *boundFile) Write(p []byte) (int, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.Write(f.ctx, p)
}

func (f *boundFile) WriteAt(p []byte, off int64) (int, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.WriteAt(f.ctx, p, off)
}

func (f *boundFile) Seek(off int64, whence int) (int64, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.Seek(f.ctx, off, whence)
}

func (f *boundFile) ReadDir(n int) ([]iofs.DirEntry, error) {
	if err := f.checkValid(); err != nil {
		return nil, err
	}
	return f.file.ReadDir(f.ctx, n)
}

func (f *boundFile) ReadFrom(r io.Reader) (int64, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.ReadFrom(f.ctx, r)
}

func (f *boundFile) WriteTo(w io.Writer) (int64, error) {
	if err := f.checkValid(); err != nil {
		return 0, err
	}
	return f.file.WriteTo(f.ctx, w)
}

type contextReader struct {
	ctx  context.Context
	file *File
}

func (r *contextReader) Read(p []byte) (int, error) { return r.file.Read(r.ctx, p) }

type contextWriter struct {
	ctx  context.Context
	file *File
}

func (w *contextWriter) Write(p []byte) (int, error) { return w.file.Write(w.ctx, p) }

var (
	_ iofs.FS          = (*boundShare)(nil)
	_ iofs.StatFS      = (*boundShare)(nil)
	_ iofs.ReadFileFS  = (*boundShare)(nil)
	_ iofs.ReadDirFS   = (*boundShare)(nil)
	_ iofs.GlobFS      = (*boundShare)(nil)
	_ iofs.ReadLinkFS  = (*boundShare)(nil)
	_ iofs.SubFS       = (*boundShare)(nil)
	_ iofs.File        = (*boundFile)(nil)
	_ iofs.ReadDirFile = (*boundFile)(nil)
	_ io.Reader        = (*boundFile)(nil)
	_ io.ReaderAt      = (*boundFile)(nil)
	_ io.Writer        = (*boundFile)(nil)
	_ io.WriterAt      = (*boundFile)(nil)
	_ io.Seeker        = (*boundFile)(nil)
	_ io.Closer        = (*boundFile)(nil)
	_ io.ReaderFrom    = (*boundFile)(nil)
	_ io.WriterTo      = (*boundFile)(nil)
)
