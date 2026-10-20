// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

// fsstatAllowedSymbols lists every "importpath.Symbol" that may be used by
// non-test Go files in allowedpaths/internal/fsstat/. This package is kept
// separate from the broader allowedpaths allowlist because its platform
// backends use narrowly audited native filesystem-query APIs.
//
// Each symbol must have a comment explaining what it does and why it is safe
// for the filesystem-stat backend. In particular, unsafe.Pointer is limited to
// passing fixed, bounded buffers to the read-only Windows native query.
var fsstatAllowedSymbols = []string{
	"encoding/binary.LittleEndian", // 🟢 decodes bounded little-endian fields returned by Windows volume-information queries; pure value.
	"errors.Is",                    // 🟢 compares the Darwin open error with EACCES; pure error-chain inspection.
	"errors.New",                   // 🟢 constructs package sentinel errors; pure function, no I/O.
	"fmt.Errorf",                   // 🟢 constructs validation errors for malformed filesystem size data; pure formatting.
	"fmt.Sprintf",                  // 🟢 formats the fallback Linux filesystem magic name; pure formatting.
	"os.ErrInvalid",                // 🟢 sentinel used to reject a non-local Windows path before native calls; pure value.
	"os.FileInfo",                  // 🟢 metadata interface used for descriptor/path identity validation; no I/O by itself.
	"os.ModeSymlink",               // 🟢 mode bit used to reject raced final symlinks; pure constant.
	"os.PathError",                 // 🟢 wraps backend errors with the caller-visible operation and path; pure data type.
	"os.Root",                      // 🟠 pinned sandbox root through which every operand and fallback parent is opened.
	"os.SameFile",                  // 🟢 compares already-collected metadata identities to detect path replacement; no new I/O.
	"path/filepath.Clean",          // 🟢 normalizes an already-root-relative path before safe parent/handle lookup.
	"path/filepath.Dir",            // 🟢 derives the parent of an already-root-relative Darwin operand; pure path manipulation.
	"path/filepath.IsLocal",        // 🟢 rejects absolute and escaping Windows paths before a relative native open; pure validation.
	"syscall.EACCES",               // 🟢 Darwin permission-denied sentinel selecting the metadata-only parent fallback.
	"syscall.Stat_t",               // 🟢 Darwin stat payload used only to compare the target and parent device identifiers.
	"unsafe.Pointer",               // 🔴 Windows only: passes fixed buffers/structs to NtQueryVolumeInformationFile; no pointer arithmetic and every decoded field is length-checked.
	"unsafe.Sizeof",                // 🟢 computes compile-time native structure sizes for bounded Windows syscall buffers and attributes.
	"golang.org/x/sys/unix.ByteSliceToString",                   // 🟢 converts Darwin's fixed NUL-terminated filesystem type buffer to a Go string.
	"golang.org/x/sys/unix.Fstatfs",                             // 🟠 queries filesystem counters through an already sandbox-confined descriptor; no path lookup or mutation.
	"golang.org/x/sys/unix.O_DIRECTORY",                         // 🟢 Darwin flag component requiring the O_SEARCH fallback handle to name a directory.
	"golang.org/x/sys/unix.O_EVTONLY",                           // 🟢 Darwin metadata/event-only open flag; prevents data reads from the target.
	"golang.org/x/sys/unix.O_NONBLOCK",                          // 🟢 Darwin flag preventing metadata opens from blocking on FIFOs and other special files.
	"golang.org/x/sys/unix.O_PATH",                              // 🟢 Linux metadata-only open flag; obtains an object handle without data-read permission.
	"golang.org/x/sys/unix.Statfs_t",                            // 🟢 fixed kernel structure carrying filesystem counters returned by fstatfs.
	"golang.org/x/sys/windows.ByHandleFileInformation",          // 🟢 fixed metadata structure used only as a volume-ID fallback.
	"golang.org/x/sys/windows.CloseHandle",                      // 🟠 releases the native metadata handle acquired for the Windows query.
	"golang.org/x/sys/windows.FILE_ATTRIBUTE_DIRECTORY",         // 🟢 attribute bit used on the already-confined handle to enforce caller-supplied directory syntax.
	"golang.org/x/sys/windows.FILE_OPEN",                        // 🟢 native open disposition selecting an existing object only.
	"golang.org/x/sys/windows.FILE_OPEN_FOR_BACKUP_INTENT",      // 🟢 permits metadata-only directory handles; the root handle and OBJ_DONT_REPARSE still enforce containment.
	"golang.org/x/sys/windows.FILE_READ_ATTRIBUTES",             // 🟢 least-privilege access right needed for read-only volume metadata.
	"golang.org/x/sys/windows.FILE_SHARE_DELETE",                // 🟢 share flag avoiding unnecessary interference with concurrent deletion.
	"golang.org/x/sys/windows.FILE_SHARE_READ",                  // 🟢 share flag avoiding unnecessary interference with concurrent readers.
	"golang.org/x/sys/windows.FILE_SHARE_WRITE",                 // 🟢 share flag avoiding unnecessary interference with concurrent writers.
	"golang.org/x/sys/windows.FILE_SYNCHRONOUS_IO_NONALERT",     // 🟢 requests synchronous completion so native query results are final on return.
	"golang.org/x/sys/windows.GetFileInformationByHandle",       // 🟠 reads identity metadata from the already-confined handle as a volume-ID fallback.
	"golang.org/x/sys/windows.Handle",                           // 🟢 opaque native handle type; carries no capability without the audited APIs below.
	"golang.org/x/sys/windows.IO_STATUS_BLOCK",                  // 🟢 fixed result structure used to bound native volume-information decoding.
	"golang.org/x/sys/windows.InvalidHandle",                    // 🟢 sentinel returned when a native metadata open cannot be constructed.
	"golang.org/x/sys/windows.NewLazySystemDLL",                 // 🔴 loads the fixed Windows system library ntdll.dll for the audited read-only volume query.
	"golang.org/x/sys/windows.NewNTUnicodeString",               // 🟢 encodes the already-validated root-relative path for NtCreateFile.
	"golang.org/x/sys/windows.NtCreateFile",                     // 🟠 opens a metadata-only handle relative to the pinned AllowedPaths root.
	"golang.org/x/sys/windows.NTStatus",                         // 🟢 native status value used to translate query failures into ordinary errors.
	"golang.org/x/sys/windows.OBJECT_ATTRIBUTES",                // 🟢 fixed native structure binding NtCreateFile to the pinned root handle.
	"golang.org/x/sys/windows.OBJ_CASE_INSENSITIVE",             // 🟢 matches ordinary Windows path lookup semantics for the relative metadata open.
	"golang.org/x/sys/windows.OBJ_DONT_REPARSE",                 // 🟢 rejects reparse points in every component so a raced path cannot escape the pinned root.
	"golang.org/x/sys/windows.STATUS_REPARSE_POINT_ENCOUNTERED", // 🟢 status sentinel that converts a raced reparse point into a safe retry.
	"golang.org/x/sys/windows.STATUS_SUCCESS",                   // 🟢 success sentinel checked before any native result buffer is decoded.
	"golang.org/x/sys/windows.SYNCHRONIZE",                      // 🟢 least-privilege right required by the synchronous native metadata handle.
	"golang.org/x/sys/windows.UTF16ToString",                    // 🟢 decodes only the explicitly length-bounded filesystem type returned by the kernel.
}
