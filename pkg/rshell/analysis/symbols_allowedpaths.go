// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

// allowedpathsAllowedSymbols lists every "importpath.Symbol" that may be used
// by non-test Go files in allowedpaths/. Each entry must be in
// "importpath.Symbol" form, where importpath is the full Go import path.
//
// Each symbol must have a comment explaining what it does and why it is safe
// to use inside the filesystem sandbox.
//
// Internal module imports (github.com/DataDog/datadog-agent/pkg/rshell/*) are auto-allowed
// and do not appear here.
//
// The permanently banned packages (reflect, unsafe) apply here too.
var allowedpathsAllowedSymbols = []string{
	"bytes.Buffer",                       // 🟢 in-memory byte buffer; collects sandbox warnings for deferred output.
	"context.Context",                    // 🟢 context type used to signal cancellation; no I/O or side effects.
	"errors.As",                          // 🟢 error type assertion; pure function, no I/O.
	"errors.Is",                          // 🟢 error comparison; pure function, no I/O.
	"errors.New",                         // 🟢 creates a simple error value; pure function, no I/O.
	"fmt.Errorf",                         // 🟢 formatted error creation; pure function, no I/O.
	"fmt.Fprintf",                        // 🟠 writes warning messages to in-memory buffer during sandbox construction.
	"golang.org/x/sys/unix.FD_SETSIZE",   // 🟢 maximum descriptor supported by Darwin select; pure constant.
	"golang.org/x/sys/unix.FdSet",        // 🟢 descriptor set used for a non-consuming Darwin FIFO readiness probe.
	"golang.org/x/sys/unix.POLLHUP",      // 🟢 poll event constant indicating that a FIFO writer disconnected.
	"golang.org/x/sys/unix.POLLIN",       // 🟢 poll event constant indicating readable FIFO data.
	"golang.org/x/sys/unix.POLLNVAL",     // 🟢 poll event constant indicating an invalid descriptor.
	"golang.org/x/sys/unix.Poll",         // 🟠 non-consuming FIFO readiness check on non-Darwin Unix systems.
	"golang.org/x/sys/unix.PollFd",       // 🟢 descriptor data passed to unix.Poll.
	"golang.org/x/sys/unix.Select",       // 🟠 non-consuming Darwin FIFO readiness check.
	"golang.org/x/sys/unix.Timeval",      // 🟢 zero timeout value used for non-blocking select.
	"io.EOF",                             // 🟢 sentinel error value; pure constant.
	"io.ReadWriteCloser",                 // 🟢 combined interface type; no side effects.
	"io.Writer",                          // 🟢 interface type; parameter type for the chunked-write-with-cancellation helper backing WriteRegularFile; no side effects by itself.
	"io/fs.DirEntry",                     // 🟢 interface type for directory entries; no side effects.
	"io/fs.ModeSymlink",                  // 🟢 file mode bit for symlinks; pure constant.
	"io/fs.ErrExist",                     // 🟢 sentinel error for "already exists"; pure constant.
	"io/fs.ErrNotExist",                  // 🟢 sentinel error for "does not exist"; pure constant.
	"io/fs.ErrPermission",                // 🟢 sentinel error for permission denied; pure constant.
	"io/fs.FileInfo",                     // 🟢 interface type for file metadata; no side effects.
	"io/fs.FileMode",                     // 🟢 file permission bits type; pure type.
	"io/fs.ModeNamedPipe",                // 🟢 file mode bit for identifying FIFO handles after an atomic non-blocking open.
	"io/fs.ReadDirFile",                  // 🟢 read-only directory handle interface; no write capability.
	"os.DevNull",                         // 🟢 platform null device path constant; pure constant.
	"os.ErrNotExist",                     // 🟢 sentinel error for missing literal paths; pure constant.
	"os.ErrPermission",                   // 🟢 sentinel error for permission denied; pure constant.
	"os.FileInfo",                        // 🟢 interface type for file metadata (alias of io/fs.FileInfo); no side effects. Used in Stat forwarding signatures so callers that type-assert against os.FileInfo (matching os.File.Stat's own return type) see through wrapper types.
	"os.File",                            // 🟠 file handle returned by os.Root.Open; needed for cross-root symlink fallback.
	"os.FileMode",                        // 🟢 file permission bits type; pure type.
	"os.Getgid",                          // 🟠 returns the numeric group id of the caller; read-only syscall.
	"os.Getgroups",                       // 🟠 returns supplementary group ids; read-only syscall.
	"os.Getuid",                          // 🟠 returns the numeric user id of the caller; read-only syscall.
	"os.IsPathSeparator",                 // 🟢 recognizes native path separators while preserving raw stat operand components; pure function.
	"os.Lstat",                           // 🟠 checks whether a literal AllowedPaths entry exists without following symlinks, preserving legacy paths that end in :ro/:rw.
	"os.O_APPEND",                        // 🟢 append-on-write file flag constant; pure constant. Part of the sandbox open-flag allowlist.
	"os.O_CREATE",                        // 🟢 create-if-missing file flag constant; pure constant. Part of the sandbox open-flag allowlist.
	"os.O_RDONLY",                        // 🟢 read-only file flag constant; pure constant.
	"os.O_TRUNC",                         // 🟢 truncate-on-open file flag constant; pure constant. Part of the sandbox open-flag allowlist.
	"os.O_WRONLY",                        // 🟢 write-only file flag constant; pure constant. Part of the sandbox open-flag allowlist.
	"os.OpenRoot",                        // 🟠 opens a directory as a root for sandboxed file access; needed for sandbox.
	"os.PathError",                       // 🟢 error type wrapping path and operation; pure type.
	"os.Root",                            // 🟠 sandboxed directory root type; core of the filesystem sandbox.
	"os.SameFile",                        // 🟢 compares metadata identities before and after a regular-file open; pure function.
	"os.Stat",                            // 🟠 returns file info for a path; needed for sandbox path validation.
	"path/filepath.Abs",                  // 🟢 returns absolute path; pure path computation.
	"path/filepath.Clean",                // 🟢 normalizes a path; pure function, no I/O.
	"path/filepath.Dir",                  // 🟢 returns directory portion of a path; pure function, no I/O.
	"path/filepath.EvalSymlinks",         // 🟠 resolves symlinks via os.Lstat; the sandbox uses this at setup time to record canonical root paths so builtins like `pwd -P` can reflect the symlink resolution that os.Root has implicitly followed.
	"path/filepath.IsAbs",                // 🟢 checks if path is absolute; pure function, no I/O.
	"path/filepath.Join",                 // 🟢 joins path elements; pure function, no I/O.
	"path/filepath.Rel",                  // 🟢 returns relative path; pure path computation.
	"path/filepath.Separator",            // 🟢 OS path separator constant; pure constant.
	"path/filepath.VolumeName",           // 🟢 extracts a Windows drive or UNC prefix before raw stat operand component traversal; pure function.
	"slices.SortFunc",                    // 🟢 sorts a slice with a comparison function; pure function, no I/O.
	"sync.Once",                          // 🟢 ensures one-time execution; used to close file descriptors at most once.
	"strings.Compare",                    // 🟢 compares two strings lexicographically; pure function, no I/O.
	"strings.ContainsRune",               // 🟢 validates remote path grant syntax without filesystem access.
	"strings.EqualFold",                  // 🟢 case-insensitive string comparison; pure function, no I/O.
	"strings.HasPrefix",                  // 🟢 pure function for prefix matching; no I/O.
	"strings.HasSuffix",                  // 🟢 pure function for suffix matching; no I/O.
	"strings.Join",                       // 🟢 joins string slices; pure function, no I/O.
	"strings.Split",                      // 🟢 splits a string by separator; pure function, no I/O.
	"syscall.ByHandleFileInformation",    // 🟢 Windows file identity structure; pure type for file metadata.
	"syscall.EINVAL",                     // 🟢 "invalid argument" errno constant; pure constant. Used by Sandbox.Truncate to reject negative sizes.
	"syscall.EISDIR",                     // 🟢 "is a directory" errno constant; pure constant.
	"syscall.Errno",                      // 🟢 system call error number type; pure type.
	"syscall.GetFileInformationByHandle", // 🟠 Windows API for file identity (vol serial + file index); read-only syscall.
	"syscall.Handle",                     // 🟢 Windows file handle type; pure type alias.
	"syscall.EAGAIN",                     // 🟢 would-block errno used to retry a non-blocking FIFO read without treating it as failure.
	"syscall.EINTR",                      // 🟢 interrupted-syscall errno used to retry FIFO readiness probes.
	"syscall.O_NONBLOCK",                 // 🟢 non-blocking open flag; prevents blocking on FIFOs during access checks. Pure constant.
	"syscall.SetNonblock",                // 🟠 restores blocking reads after a sandboxed open proves the target is not a FIFO.
	"syscall.Stat_t",                     // 🟢 file stat structure type; pure type for Unix file metadata.
	"time.Millisecond",                   // 🟢 duration constant used to pace non-blocking FIFO read retries.
	"time.NewTicker",                     // 🟢 creates an in-process ticker so unattached FIFO reads wait without spinning.
}
