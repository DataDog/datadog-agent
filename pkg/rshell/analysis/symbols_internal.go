// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

// internalPerPackageSymbols maps each builtins/internal/<package> name to the
// symbols it is allowed to use. Every symbol listed here must also appear in
// internalAllowedSymbols (which acts as the global ceiling).
var internalPerPackageSymbols = map[string][]string{
	"diskstats": {
		"bufio.ErrTooLong",     // 🟢 sentinel error for scanner buffer overflow; pure constant.
		"bufio.NewScanner",     // 🟢 line-by-line reading of /proc/self/mountinfo; no write capability.
		"context.Context",      // 🟢 deadline/cancellation interface; no side effects.
		"errors.Is",            // 🟢 checks whether an error in a chain matches a target; pure function, no I/O.
		"errors.New",           // 🟢 creates a sentinel error (ErrNotSupported, ErrMaxMounts, ErrLineTooLong); pure function, no I/O.
		"fmt.Errorf",           // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",          // 🟢 string formatting; used to encode Statfs_t.Fsid as "major:minor"; pure function, no I/O.
		"io.Reader",            // 🟢 interface type used to feed parseMountInfo from arbitrary readers (tests use strings.NewReader); pure type, no I/O.
		"os.Open",              // 🟠 opens /proc/self/mountinfo read-only. Bypasses AllowedPaths by design — the path is hardcoded and never derived from user input, mirroring procnetsocket's documented exception.
		"strings.Builder",      // 🟢 in-memory buffer for octal-escape unescape of mountinfo paths; no I/O.
		"strings.Contains",     // 🟢 checks for ":" in a mount source string to detect host:/export remote mounts; pure function, no I/O.
		"strings.ContainsRune", // 🟢 fast-path check for backslash before unescape; pure function, no I/O.
		"strings.Cut",          // 🟢 splits a string at the first separator; pure function, no I/O.
		"strings.Fields",       // 🟢 splits whitespace-separated mountinfo fields; pure function, no I/O.
		"strings.HasPrefix",    // 🟢 checks remote-FS-type prefix and "//" UNC source prefix; pure function, no I/O.
		"golang.org/x/sys/unix.ByteSliceToString", // 🟢 converts a NUL-terminated kernel byte buffer to a Go string; pure function, no I/O.
		"golang.org/x/sys/unix.Getfsstat",         // 🟠 (darwin) read-only enumeration of mounted filesystems via getfsstat(2); no exec or write capability.
		"golang.org/x/sys/unix.MNT_LOCAL",         // 🟢 (darwin) flag constant indicating a local-only filesystem; pure constant.
		"golang.org/x/sys/unix.MNT_NOWAIT",        // 🟢 (darwin) flag constant: do not block on remote FS for getfsstat; pure constant.
		"golang.org/x/sys/unix.Statfs",            // 🟠 (linux) read-only filesystem usage syscall; no exec or write capability.
		"golang.org/x/sys/unix.Statfs_t",          // 🟢 struct type carrying filesystem usage data from statfs/getfsstat; pure data type.
	},
	"loopctl": {
		"strconv.Atoi", // 🟢 string-to-int conversion; pure function, no I/O.
	},
	"meminfo": {
		"bufio.NewScanner",  // 🟢 line-by-line reading of /proc/meminfo; no write capability.
		"context.Context",   // 🟢 deadline/cancellation interface; no side effects.
		"errors.New",        // 🟢 creates a sentinel error (ErrNotSupported); pure function, no I/O.
		"fmt.Errorf",        // 🟢 error formatting; pure function, no I/O.
		"io.Reader",         // 🟢 interface type used to feed parseMeminfo from arbitrary readers (tests use strings.NewReader); pure type, no I/O.
		"os.Open",           // 🟠 opens /proc/meminfo read-only. Bypasses AllowedPaths by design — the path is hardcoded and never derived from user input, mirroring diskstats's documented exception.
		"strconv.ParseUint", // 🟢 parses the numeric KiB value out of each meminfo line; pure function, no I/O.
		"strings.Cut",       // 🟢 splits "Key:    Value kB" at the first colon; pure function, no I/O.
		"strings.CutSuffix", // 🟢 strips the trailing " kB" unit before parsing the number; pure function, no I/O.
		"strings.TrimSpace", // 🟢 trims whitespace between the colon and the value; pure function, no I/O.
	},
	"procinfo": {
		"strconv.FormatUint",           // 🟢 formats process counters; pure function, no I/O.
		"bufio.NewScanner",             // 🟢 line-by-line reading of /proc files; no write capability.
		"bytes.NewReader",              // 🟢 wraps a byte slice as an in-memory io.Reader; no I/O side effects.
		"encoding/binary.LittleEndian", // 🟢 decodes fields from Darwin's fixed-size proc_taskinfo buffer; pure byte conversion, no I/O.
		"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath.Default", // 🟢 canonical /proc filesystem root path constant; pure constant, no I/O.
		"context.Context",                       // 🟢 deadline/cancellation interface; no side effects.
		"errors.Is",                             // 🟢 checks whether an error in a chain matches a target; pure function, no I/O.
		"errors.New",                            // 🟢 creates a sentinel error (unsupported-platform stub); pure function, no I/O.
		"fmt.Errorf",                            // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                           // 🟢 string formatting; pure function, no I/O.
		"io.LimitReader",                        // 🟢 caps each Linux proc pseudo-file read before buffering; pure wrapper, no I/O by itself.
		"io.ReadAll",                            // 🟠 buffers only proc data already bounded by io.LimitReader.
		"math/bits.Div64",                       // 🟢 overflow-safe division for Darwin CPU tick conversion; pure arithmetic.
		"math/bits.Mul64",                       // 🟢 overflow-safe multiplication for Darwin CPU tick conversion; pure arithmetic.
		"os.ErrNotExist",                        // 🟢 sentinel error value indicating a file or directory does not exist; read-only constant, no I/O.
		"os.Getpagesize",                        // 🟢 returns the host page size for Linux RSS conversion; read-only, no I/O.
		"os.Getpid",                             // 🟠 returns the current process ID; read-only, no side effects.
		"os.Open",                               // 🟠 opens a file read-only; needed to stream /proc/stat line-by-line.
		"os.ReadDir",                            // 🟠 reads a directory listing; needed to enumerate /proc entries.
		"os.ReadFile",                           // 🟠 reads a whole file; needed to read /proc/[pid]/{stat,status}.
		"os.Stat",                               // 🟠 validates that the proc path exists before enumeration; read-only metadata, no write capability.
		"path/filepath.Join",                    // 🟢 joins path elements to construct /proc/<pid>/stat paths; pure function, no I/O.
		"runtime.KeepAlive",                     // 🟠 keeps Go buffers live through Windows read-only syscall/DLL ABI calls; no I/O itself.
		"strconv.Atoi",                          // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.Itoa",                          // 🟢 int-to-string conversion for PID directory names; pure function, no I/O.
		"strconv.ParseInt",                      // 🟢 string to int64 with base/bit-size; pure function, no I/O.
		"strconv.ParseUint",                     // 🟢 parses unsigned Linux /proc resource counters; pure function, no I/O.
		"strings.Fields",                        // 🟢 splits a string on whitespace; pure function, no I/O.
		"strings.HasPrefix",                     // 🟢 checks string prefix; pure function, no I/O.
		"strings.Index",                         // 🟢 finds first occurrence of a substring; pure function, no I/O.
		"strings.LastIndex",                     // 🟢 finds last occurrence of a substring; pure function, no I/O.
		"strings.TrimSpace",                     // 🟢 removes leading/trailing whitespace; pure function, no I/O.
		"syscall.Getsid",                        // 🟠 returns the session ID of a process; read-only syscall, no write/exec.
		"syscall.SYS_PROC_INFO",                 // 🟢 Darwin trap number for proc_pidinfo(PROC_PIDTASKINFO); pure constant.
		"syscall.Syscall6",                      // 🟠 invokes Darwin proc_pidinfo for read-only task counters into a fixed-size buffer.
		"time.Duration",                         // 🟢 duration type for CPU and elapsed metrics; pure integer alias, no I/O.
		"time.Now",                              // 🟠 returns the current wall-clock time; read-only, no side effects.
		"time.ParseDuration",                    // 🟢 parses bounded Linux /proc uptime text into a duration; pure function, no I/O.
		"time.Second",                           // 🟢 duration constant for tick/time conversions; no side effects.
		"time.Time",                             // 🟢 process start-time value type; pure data, no side effects.
		"time.Unix",                             // 🟢 constructs a Time from Unix seconds; pure function, no I/O.
		"unicode.IsGraphic",                     // 🟢 reports whether a process-name rune is printable; pure function, no I/O.
		"unicode/utf8.DecodeRuneInString",       // 🟢 decodes process names without splitting UTF-8 sequences; pure function, no I/O.
		"unicode/utf8.RuneError",                // 🟢 replacement rune constant used to detect malformed UTF-8; no side effects.
		"unsafe.Alignof",                        // 🟢 reports the Windows ABI record alignment; compile-time layout query, no memory access.
		"unsafe.Pointer",                        // 🔴 passes fixed or bounded buffers to Darwin/Windows read-only ABIs; slice bounds are validated before typed access.
		"unsafe.Sizeof",                         // 🟢 reports fixed Windows ABI structure sizes; compile-time layout query, no memory access.
		"golang.org/x/sys/unix.KinfoProc",       // 🟢 (darwin) struct type carrying per-process kinfo_proc data from sysctl; read-only data, no exec capability.
		"golang.org/x/sys/unix.SysctlKinfoProc", // 🟠 (darwin) reads a single process's kinfo_proc via kern.proc.pid sysctl; read-only, no exec or write capability.
		"golang.org/x/sys/unix.SysctlKinfoProcSlice", // 🟠 (darwin) reads all processes' kinfo_proc via kern.proc.all sysctl; read-only, no exec or write capability.
		"golang.org/x/sys/unix.SysctlUint64",         // 🟠 (darwin) reads hw.memsize/hw.tbfrequency for resource-metric conversion; read-only sysctl.

		"golang.org/x/sys/windows.CloseHandle",              // 🟠 (windows) closes a process-snapshot handle after enumeration; no data read or exec capability.
		"golang.org/x/sys/windows.CreateToolhelp32Snapshot", // 🟠 (windows) creates a read-only snapshot of the process table; no exec or write capability.
		"golang.org/x/sys/windows.ERROR_NO_MORE_FILES",      // 🟢 (windows) sentinel error indicating end of process enumeration; pure constant.
		"golang.org/x/sys/windows.Process32First",           // 🟠 (windows) reads the first entry from a process snapshot; read-only, no exec capability.
		"golang.org/x/sys/windows.Process32Next",            // 🟠 (windows) advances to the next entry in a process snapshot; read-only, no exec capability.
		"golang.org/x/sys/windows.ProcessEntry32",           // 🟢 (windows) struct type holding process snapshot entry data; pure data type, no I/O.
		"golang.org/x/sys/windows.TH32CS_SNAPPROCESS",       // 🟢 (windows) flag constant selecting process entries for CreateToolhelp32Snapshot; pure constant.
		"golang.org/x/sys/windows.UTF16ToString",            // 🟢 (windows) converts a null-terminated UTF-16 slice to a Go string; pure function, no I/O.

		"golang.org/x/sys/windows.Filetime",                          // 🟢 (windows) fixed-layout timestamp data returned by GetProcessTimes; no I/O.
		"golang.org/x/sys/windows.GetProcessTimes",                   // 🟠 (windows) reads creation and CPU times from a query-only process handle.
		"golang.org/x/sys/windows.NewLazySystemDLL",                  // 🔴 (windows) resolves the fixed system kernel32.dll for read-only GlobalMemoryStatusEx.
		"golang.org/x/sys/windows.NtQuerySystemInformation",          // 🔴 (windows) reads a kernel process snapshot into a buffer capped at 32 MiB; no write/exec capability.
		"golang.org/x/sys/windows.OpenProcess",                       // 🟠 (windows) opens a process with query-limited rights only for GetProcessTimes.
		"golang.org/x/sys/windows.PROCESS_QUERY_LIMITED_INFORMATION", // 🟢 (windows) query-only OpenProcess access-right constant; pure constant.
		"golang.org/x/sys/windows.STATUS_INFO_LENGTH_MISMATCH",       // 🟢 (windows) retry sentinel when the bounded process snapshot buffer is too small.
		"golang.org/x/sys/windows.SYSTEM_PROCESS_INFORMATION",        // 🔴 (windows) kernel ABI record type parsed only after alignment and bounds checks.
		"golang.org/x/sys/windows.SystemProcessInformation",          // 🟢 (windows) NtQuerySystemInformation class selecting read-only process data.
	},
	"procpath": {
		// No stdlib symbols needed — this package only defines a string constant.
	},
	"procfd": {
		"bufio.NewScanner",               // 🟢 line-by-line reading of /proc/<pid>/status for the Uid field; no write capability.
		"bytes.NewReader",                // 🟢 wraps a byte slice as an in-memory io.Reader; no I/O side effects.
		"context.Context",                // 🟢 deadline/cancellation interface; no side effects.
		"errors.New",                     // 🟢 creates a sentinel error (ErrNotSupported); pure function, no I/O.
		"fmt.Errorf",                     // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                    // 🟢 formats the "major,minor" device string; pure function, no I/O.
		"os.Open",                        // 🟠 opens a process's /proc/<pid>/fd directory read-only to stream its entries in bounded batches; no write capability.
		"os.ReadDir",                     // 🟠 enumerates /proc PIDs; read-only directory listing.
		"os.ReadFile",                    // 🟠 reads /proc/<pid>/stat and /proc/<pid>/status; read-only metadata, no write capability.
		"os.Readlink",                    // 🟠 resolves an fd/cwd/root/exe magic symlink's kernel-reported target; read-only, no write capability.
		"os.Stat",                        // 🟠 validates procPath is accessible before scanning explicit -p PIDs; read-only, no write capability.
		"path/filepath.Join",             // 🟢 joins path elements to construct /proc/<pid>/fd/<n> paths; pure function, no I/O.
		"slices.Sort",                    // 🟢 sorts the scanned PID list ascending; pure function, no I/O.
		"slices.SortFunc",                // 🟢 sorts numeric fds ascending by their parsed int value; pure function, no I/O.
		"strconv.Atoi",                   // 🟢 string-to-int conversion for PID and fd directory names; pure function, no I/O.
		"strconv.FormatUint",             // 🟢 formats the inode number as a decimal string; pure function, no I/O.
		"strconv.FormatInt",              // 🟢 formats the file size as a decimal string; pure function, no I/O.
		"strconv.Itoa",                   // 🟢 formats a PID as a directory name; pure function, no I/O.
		"strings.CutSuffix",              // 🟢 strips the kernel's " (deleted)" marker from a readlink target; pure function, no I/O.
		"strings.Fields",                 // 🟢 splits the /proc/<pid>/status "Uid:" line on whitespace; pure function, no I/O.
		"strings.HasPrefix",              // 🟢 checks for the "Uid:" line prefix and for absolute-path/memfd targets; pure function, no I/O.
		"strings.Index",                  // 🟢 finds the comm field's opening '(' in /proc/<pid>/stat; pure function, no I/O.
		"strings.LastIndex",              // 🟢 finds the comm field's closing ')' in /proc/<pid>/stat; pure function, no I/O.
		"strings.TrimSpace",              // 🟢 trims whitespace around /proc/<pid>/stat contents; pure function, no I/O.
		"golang.org/x/sys/unix.Major",    // 🟢 extracts the major device number from a raw dev_t; pure function, no I/O.
		"golang.org/x/sys/unix.Minor",    // 🟢 extracts the minor device number from a raw dev_t; pure function, no I/O.
		"golang.org/x/sys/unix.S_IFBLK",  // 🟢 file-type bitmask constant for block devices; pure constant.
		"golang.org/x/sys/unix.S_IFCHR",  // 🟢 file-type bitmask constant for character devices; pure constant.
		"golang.org/x/sys/unix.S_IFDIR",  // 🟢 file-type bitmask constant for directories; pure constant.
		"golang.org/x/sys/unix.S_IFIFO",  // 🟢 file-type bitmask constant for FIFOs; pure constant.
		"golang.org/x/sys/unix.S_IFLNK",  // 🟢 file-type bitmask constant for symlinks; pure constant.
		"golang.org/x/sys/unix.S_IFMT",   // 🟢 file-type bitmask mask constant; pure constant.
		"golang.org/x/sys/unix.S_IFREG",  // 🟢 file-type bitmask constant for regular files; pure constant.
		"golang.org/x/sys/unix.S_IFSOCK", // 🟢 file-type bitmask constant for sockets; pure constant.
		"golang.org/x/sys/unix.Stat",     // 🟠 stats an fd's magic symlink path to get Type/Device/Size/Node, including for deleted-but-open files; read-only, no write capability.
		"golang.org/x/sys/unix.Stat_t",   // 🟢 struct type carrying stat(2) data; pure data type.
	},
	"sizeparse": {
		"errors.New",       // 🟢 creates sentinel parse errors; pure function, no I/O.
		"strconv.ParseInt", // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
	},
	"procsyskernel": {
		"fmt.Errorf",          // 🟢 error formatting; pure function, no I/O.
		"io.LimitReader",      // 🟢 wraps a reader with a byte cap; pure wrapper, no I/O by itself.
		"io.ReadAll",          // 🟠 reads all data from a reader; bounded by io.LimitReader.
		"os.ModeCharDevice",   // 🟢 file mode constant; pure constant.
		"os.O_RDONLY",         // 🟢 read-only file flag; pure constant.
		"os.OpenFile",         // 🟠 opens kernel pseudo-files for reading; bypasses AllowedPaths by design.
		"path/filepath.Base",  // 🟢 returns the last element of a path; validates name is a plain basename.
		"path/filepath.Clean", // 🟢 normalises path before use; pure function, no I/O.
		"path/filepath.Join",  // 🟢 joins path elements; pure function, no I/O.
		"strings.Contains",    // 🟢 checks for ".." traversal in procPath; pure function, no I/O.
		"strings.TrimRight",   // 🟢 trims trailing characters; pure function, no I/O.
		"syscall.O_NONBLOCK",  // 🟢 non-blocking open flag; prevents FIFO hang. Pure constant.
	},
	"procnetroute": {
		"bufio.NewScanner", // 🟢 line-by-line reading of /proc/net/route; no write capability.
		"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath.Default", // 🟢 canonical /proc filesystem root path constant; pure constant, no I/O.
		"context.Context",          // 🟢 deadline/cancellation interface; no side effects.
		"errors.New",               // 🟢 creates a sentinel error (non-Linux stub); pure function, no I/O.
		"fmt.Errorf",               // 🟢 error formatting for unsafe-path guard; pure function, no I/O.
		"fmt.Sprintf",              // 🟢 formats dotted-decimal IP strings; pure function, no I/O.
		"math/bits.OnesCount32",    // 🟢 counts set bits in a uint32 (popcount for prefix length); pure function, no I/O.
		"math/bits.ReverseBytes32", // 🟢 byte-swaps a uint32 to convert little-endian /proc mask to network byte order for CIDR validation; pure function, no I/O.
		"os.Open",                  // 🟠 opens /proc/net/route read-only; needed to stream the routing table.
		"path/filepath.Clean",      // 🟢 cleans procPath before ".." component check; pure function, no I/O.
		"path/filepath.Join",       // 🟢 joins procPath + "net/route"; pure function, no I/O.
		"strconv.ParseUint",        // 🟢 parses hex/decimal route fields; pure function, no I/O.
		"strings.Contains",         // 🟢 checks for ".." components in procPath safety guard; pure function, no I/O.
		"strings.Fields",           // 🟢 splits whitespace-separated route lines; pure function, no I/O.
	},
	"flagparser": {
		"errors.New",                             // 🟢 creates a sentinel error (noArgBool.Set rejects explicit values); pure function, no I/O.
		"github.com/spf13/pflag.ContinueOnError", // 🟢 pflag parsing-mode constant used to set up trial FlagSet; pure constant.
		"github.com/spf13/pflag.FlagSet",         // 🟢 pflag FlagSet type used to trial-parse argv prefixes; pure type, no I/O.
		"github.com/spf13/pflag.NewFlagSet",      // 🟢 constructs a throw-away FlagSet for trial-parsing; pure constructor, no I/O.
		"io.Discard",                             // 🟢 silences trial.SetOutput so trial-parse failures don't leak to stderr; pure no-op writer.
		"strconv.QuoteToGraphic",                 // 🟢 escapes control, format, and malformed bytes in option diagnostics; pure conversion.
		"strings.Cut",                            // 🟢 splits pflag error/descriptor strings at separators; pure function, no I/O.
		"strings.CutPrefix",                      // 🟢 matches pflag error prefixes when rewriting to GNU wording; pure function, no I/O.
		"strings.HasPrefix",                      // 🟢 matches pflag error prefixes; pure function, no I/O.
		"strings.HasSuffix",                      // 🟢 matches pflag error suffixes (e.g. "flag does not allow an argument"); pure function, no I/O.
		"strings.LastIndex",                      // 🟢 finds the ", " separator in pflag flag descriptors; pure function, no I/O.
	},
	"procnetsocket": {
		"bufio.NewScanner", // 🟢 line-by-line reading of /proc/net/{tcp,udp,unix}; no write capability.
		"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath.Default", // 🟢 canonical /proc filesystem root path constant; pure constant, no I/O.
		"context.Context",     // 🟢 deadline/cancellation interface; no side effects.
		"errors.New",          // 🟢 creates a sentinel error (non-Linux stub); pure function, no I/O.
		"fmt.Errorf",          // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",         // 🟢 formats dotted-decimal IP/port strings; pure function, no I/O.
		"os.Open",             // 🟠 opens /proc/net/tcp* and /proc/net/udp* read-only; needed to stream socket tables.
		"path/filepath.Clean", // 🟢 cleans procPath before ".." component check; pure function, no I/O.
		"path/filepath.Join",  // 🟢 joins procPath + "net/<file>"; pure function, no I/O.
		"strconv.FormatUint",  // 🟢 uint-to-string conversion for port/inode formatting; pure function, no I/O.
		"strconv.ParseUint",   // 🟢 parses hex/decimal socket fields; pure function, no I/O.
		"strings.Builder",     // 🟢 efficient string concatenation for IPv6 formatting; pure in-memory buffer, no I/O.
		"strings.Contains",    // 🟢 checks for ".." components in procPath safety guard; pure function, no I/O.
		"strings.Fields",      // 🟢 splits whitespace-separated socket lines; pure function, no I/O.
		"strings.Join",        // 🟢 reconstructs space-containing Unix socket paths from Fields tokens; pure function, no I/O.
		"strings.Split",       // 🟢 splits address:port fields on ":"; pure function, no I/O.
		"strings.ToUpper",     // 🟢 normalises hex state field to uppercase for map lookup; pure function, no I/O.
	},
	"vmstat": {
		"bufio.NewScanner",                   // 🟢 line-by-line reading of /proc/{stat,meminfo,vmstat,loadavg,uptime}; no write capability.
		"context.Context",                    // 🟢 deadline/cancellation interface; no side effects.
		"errors.New",                         // 🟢 creates a sentinel error (ErrNotSupported, and a stub-platform error path); pure function, no I/O.
		"fmt.Errorf",                         // 🟢 error formatting; pure function, no I/O.
		"io.EOF",                             // 🟢 sentinel error value returned by bufio.Scanner at end of file; pure constant.
		"math.IsInf",                         // 🟢 rejects infinite load-average/uptime values; pure function, no I/O.
		"math.IsNaN",                         // 🟢 rejects NaN load-average/uptime values; pure function, no I/O.
		"math.MaxUint64",                     // 🟢 integer constant; bounds the /proc/meminfo KiB-to-bytes conversion against overflow; no side effects.
		"os.Getpagesize",                     // 🟢 returns the host's memory page size; read-only, no I/O.
		"os.Open",                            // 🟠 opens /proc/{stat,meminfo,vmstat,loadavg,uptime} read-only; needed to stream kernel pseudo-files.
		"path/filepath.Join",                 // 🟢 joins procPath + file name (e.g. "stat"); pure function, no I/O.
		"strconv.ParseFloat",                 // 🟢 parses load-average and uptime float fields; pure function, no I/O.
		"strconv.ParseUint",                  // 🟢 parses /proc counter fields; pure function, no I/O.
		"strings.Cut",                        // 🟢 splits a "Key: value" meminfo line at the first colon; pure function, no I/O.
		"strings.Fields",                     // 🟢 splits whitespace-separated /proc lines; pure function, no I/O.
		"strings.HasPrefix",                  // 🟢 matches /proc/stat line prefixes (cpu/intr/ctxt/procs_*); pure function, no I/O.
		"golang.org/x/sys/unix.Getpagesize",  // 🟢 (darwin) returns the host's memory page size; read-only, no I/O.
		"golang.org/x/sys/unix.SysctlRaw",    // 🟠 (darwin) reads raw sysctl byte replies (vm.swapusage, vm.loadavg); read-only, no exec or write capability.
		"golang.org/x/sys/unix.SysctlUint64", // 🟠 (darwin) reads a uint64 sysctl value (hw.memsize); read-only, no exec or write capability.
	},
	"winnet": {
		"encoding/binary.BigEndian",    // 🟢 reads big-endian IPv6 group values from DLL buffer; pure value, no I/O.
		"encoding/binary.LittleEndian", // 🟢 reads little-endian DWORD fields from DLL buffer; pure value, no I/O.
		"errors.New",                   // 🟢 creates a sentinel error (non-Windows stub); pure function, no I/O.
		"fmt.Errorf",                   // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                  // 🟢 string formatting; pure function, no I/O.
		"syscall.Errno",                // 🟢 wraps DLL return code as an error type; pure type, no I/O.
		"syscall.MustLoadDLL",          // 🔴 loads iphlpapi.dll once at program init; read-only OS loader call.
		"syscall.Proc",                 // 🟢 DLL procedure handle type used in function signature; pure type, no I/O.
		"unsafe.Pointer",               // 🔴 passes buffer/size pointers to DLL via syscall ABI. No pointer arithmetic; buffer parsed with encoding/binary after the call.
	},
	"procmaps": {
		"bufio.NewScanner",                      // 🟢 (linux) line-by-line reading of /proc/<pid>/maps and /proc/<pid>/smaps; no write capability.
		"context.Context",                       // 🟢 deadline/cancellation interface; no side effects.
		"errors.Is",                             // 🟢 checks whether an error in a chain matches a target (os.ErrNotExist, windows.ERROR_INVALID_PARAMETER); pure function, no I/O.
		"errors.New",                            // 🟢 creates sentinel errors (ErrNotSupported, ErrExtendedNotSupported, ErrNoSuchProcess); pure function, no I/O.
		"fmt.Errorf",                            // 🟢 error formatting; pure function, no I/O.
		"io.LimitReader",                        // 🟢 (linux) bounds the trusted proc-root comm read before buffering it.
		"io.ReadAll",                            // 🟢 (linux) buffers only the explicitly limited comm reader.
		"math.MaxUint32",                        // 🟢 (windows) upper bound for a valid PID before the uint32(pid) OpenProcess cast; pure constant.
		"os.ErrNotExist",                        // 🟢 (linux) sentinel error value indicating a file or directory does not exist; read-only constant, no I/O.
		"os.Open",                               // 🟠 (linux) opens <trusted ProcPath>/<pid>/{comm,maps,smaps} read-only. Bypasses AllowedPaths by design; the proc root is fixed by the embedding application and the remainder derives only from the numeric PID.
		"path/filepath.Base",                    // 🟢 returns the last element of a path; used for file-backed mapping names and the Windows main module name.
		"path/filepath.Join",                    // 🟢 (linux) joins procPath + pid + filename to construct /proc/<pid>/{comm,maps,smaps} paths; pure function, no I/O.
		"strconv.Itoa",                          // 🟢 (linux) int-to-string conversion for the PID directory name; pure function, no I/O.
		"strconv.ParseUint",                     // 🟢 (linux) parses hex address-range and decimal smaps KB fields; pure function, no I/O.
		"strings.Fields",                        // 🟢 (linux) splits whitespace-separated maps/smaps fields; pure function, no I/O.
		"strings.HasPrefix",                     // 🟢 (linux) checks for a bracketed special mapping name and smaps field keys; pure function, no I/O.
		"strings.IndexByte",                     // 🟢 (linux) finds the '-' separator in a maps address range; pure function, no I/O.
		"strings.TrimRight",                     // 🟢 (linux) trims the trailing newline from /proc/<pid>/comm; pure function, no I/O.
		"strings.TrimSpace",                     // 🟢 (linux) trims whitespace around a recovered mapping pathname; pure function, no I/O.
		"encoding/binary.LittleEndian",          // 🟢 (darwin) parses the little-endian proc_regioninfo struct fields returned by the raw PROC_PIDREGIONINFO syscall; pure value, no I/O.
		"syscall.EPERM",                         // 🟢 (darwin) sentinel errno distinguishing "caller lacks privilege" from "end of region walk"; pure constant.
		"syscall.SYS_PROC_INFO",                 // 🟢 (darwin) raw syscall trap number for proc_pidinfo, not wrapped by golang.org/x/sys/unix; pure constant.
		"syscall.Syscall6",                      // 🟠 (darwin) invokes the proc_pidinfo(PROC_PIDREGIONINFO) kernel call directly; read-only region enumeration, no exec or write capability.
		"unsafe.Pointer",                        // 🔴 (darwin) passes a fixed-size buffer's address into the raw proc_pidinfo syscall ABI. No pointer arithmetic; buffer parsed with encoding/binary after the call.
		"golang.org/x/sys/unix.SysctlKinfoProc", // 🟠 (darwin) reads a single process's kinfo_proc via kern.proc.pid sysctl, used only for the short comm name; read-only, no exec or write capability.
		"golang.org/x/sys/windows.CloseHandle",  // 🟠 (windows) closes the process handle after enumeration; no data read or exec capability.
		"golang.org/x/sys/windows.ERROR_INVALID_PARAMETER",   // 🟢 (windows) sentinel error from OpenProcess when the PID does not name a running process; pure constant.
		"golang.org/x/sys/windows.GetModuleFileNameEx",       // 🟠 (windows) reads the main module's file path; read-only, no exec capability.
		"golang.org/x/sys/windows.Handle",                    // 🟢 (windows) opaque process handle type; pure type, no I/O.
		"golang.org/x/sys/windows.MEM_COMMIT",                // 🟢 (windows) MEMORY_BASIC_INFORMATION.State value selecting committed regions; pure constant.
		"golang.org/x/sys/windows.MemoryBasicInformation",    // 🟢 (windows) struct type carrying VirtualQueryEx region data; pure data type, no I/O.
		"golang.org/x/sys/windows.OpenProcess",               // 🟠 (windows) opens a process with query/VM-read rights only; no write or exec capability.
		"golang.org/x/sys/windows.PAGE_EXECUTE",              // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_EXECUTE_READ",         // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_EXECUTE_READWRITE",    // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_EXECUTE_WRITECOPY",    // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_GUARD",                // 🟢 (windows) PAGE_* protection modifier bit masked off before mode classification; pure constant.
		"golang.org/x/sys/windows.PAGE_NOCACHE",              // 🟢 (windows) PAGE_* protection modifier bit masked off before mode classification; pure constant.
		"golang.org/x/sys/windows.PAGE_READONLY",             // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_READWRITE",            // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PAGE_WRITECOMBINE",         // 🟢 (windows) PAGE_* protection modifier bit masked off before mode classification; pure constant.
		"golang.org/x/sys/windows.PAGE_WRITECOPY",            // 🟢 (windows) PAGE_* protection constant; pure constant.
		"golang.org/x/sys/windows.PROCESS_QUERY_INFORMATION", // 🟢 (windows) OpenProcess access-right constant; pure constant.
		"golang.org/x/sys/windows.PROCESS_VM_READ",           // 🟢 (windows) OpenProcess access-right constant; pure constant.
		"golang.org/x/sys/windows.UTF16ToString",             // 🟢 (windows) converts a null-terminated UTF-16 slice to a Go string; pure function, no I/O.
		"golang.org/x/sys/windows.VirtualQueryEx",            // 🟠 (windows) read-only enumeration of another process's virtual memory regions; no exec or write capability.
	},
	"sysinfo": {
		"encoding/binary.LittleEndian",    // 🟢 (darwin) decodes kern.boottime tv_sec and vm.loadavg fixed-point fields from sysctl buffers; pure value, no I/O.
		"errors.New",                      // 🟢 creates ErrNotSupported sentinel; pure function, no I/O.
		"fmt.Errorf",                      // 🟢 error wrapping; pure function, no I/O.
		"io.LimitReader",                  // 🟢 (linux) caps /proc/uptime and /proc/loadavg reads at 128 bytes; pure wrapper.
		"io.ReadAll",                      // 🟠 (linux) reads bounded /proc pseudo-file content; used with LimitReader.
		"os.Open",                         // 🟠 (linux) opens /proc/uptime and /proc/loadavg read-only; paths are hardcoded and never derived from user input — AllowedPaths bypass by design.
		"strconv.ParseFloat",              // 🟢 (linux) parses uptime seconds and load average values; pure function, no I/O.
		"strings.Fields",                  // 🟢 (linux) splits whitespace-separated /proc/uptime and /proc/loadavg content; pure function, no I/O.
		"syscall.MustLoadDLL",             // 🔴 (windows) loads kernel32.dll once at init for GetTickCount64; read-only OS loader call.
		"time.Now",                        // 🟠 computes boot timestamp as Now().Unix() - int64(uptimeSeconds); read-only, no side effects.
		"golang.org/x/sys/unix.SysctlRaw", // 🟠 (darwin) reads kern.boottime and vm.loadavg via sysctl(3); read-only, no exec or write capability.
	},
	"winpoll": {
		"syscall.Errno",       // 🟢 (windows) error number type for distinguishing ERROR_BROKEN_PIPE from other PeekNamedPipe failures; pure type.
		"syscall.MustLoadDLL", // 🔴 (windows) loads kernel32.dll once at program init for PeekNamedPipe; read-only OS loader call.
		"unsafe.Pointer",      // 🔴 (windows) passes &avail to PeekNamedPipe via syscall ABI for non-consuming readability probe. Single call site; no pointer arithmetic — the returned uint32 is consumed directly.
		"golang.org/x/sys/windows.ERROR_BROKEN_PIPE",             // 🟢 (windows) sentinel error indicating the pipe's writer end has closed — used to recognize EOF-ready pipes for `read -t 0` POLLHUP-equivalent semantics; pure constant.
		"golang.org/x/sys/windows.FILE_TYPE_CHAR",                // 🟢 (windows) GetFileType result for console/character devices; pure constant.
		"golang.org/x/sys/windows.FILE_TYPE_DISK",                // 🟢 (windows) GetFileType result for regular files; pure constant.
		"golang.org/x/sys/windows.FILE_TYPE_PIPE",                // 🟢 (windows) GetFileType result for anonymous and named pipes; pure constant.
		"golang.org/x/sys/windows.FILE_TYPE_REMOTE",              // 🟢 (windows) GetFileType modifier bit for remote-mounted volumes; pure constant.
		"golang.org/x/sys/windows.GetFileType",                   // 🟠 (windows) returns the type (disk/pipe/char/remote/unknown) of an open handle; read-only metadata, no I/O.
		"golang.org/x/sys/windows.GetNumberOfConsoleInputEvents", // 🟠 (windows) reports the count of queued console input events without consuming them; read-only inspection.
		"golang.org/x/sys/windows.Handle",                        // 🟢 (windows) opaque file/handle type used to call PeekNamedPipe and GetFileType; pure type.
	},
	"ntfsmft": {
		"bytes.Equal",                  // 🟢 compares a decoded ASCII extension against a wanted extension; pure function, no I/O.
		"cmp.Compare",                  // 🟢 orders $MFT $DATA segments by their LowestVcn; pure function, no I/O.
		"container/heap.Fix",           // 🟢 re-establishes the top-N min-heap invariant after replacing the root; pure in-memory.
		"container/heap.Push",          // 🟢 pushes a candidate onto the top-N min-heap; pure in-memory.
		"context.Context",              // 🟢 deadline/cancellation interface honoured between MFT chunks; no side effects.
		"encoding/binary.LittleEndian", // 🟢 decodes little-endian MFT record/attribute fields from in-memory buffers; pure value, no I/O.
		"errors.New",                   // 🟢 creates sentinel parse/volume errors (bad signature, torn write, unsupported layout); pure function, no I/O.
		"fmt.Errorf",                   // 🟢 error formatting; pure function, no I/O.
		"math.MaxInt64",                // 🟢 rejects raw NTFS size fields whose high bit is set (would wrap negative on int64 conversion); pure constant, no I/O.
		"path/filepath.Clean",          // 🟢 normalises the already-absolute scan target / exclude paths; pure function, no I/O.
		"path/filepath.IsAbs",          // 🟢 rejects relative scan target / exclude paths so the host process cwd is never consulted; pure function, no I/O.
		"path/filepath.Match",          // 🟢 evaluates a --find-glob pattern against a basename; pure function, no I/O.
		"regexp.Compile",               // 🟢 compiles a --find-regex pattern with the RE2 (linear-time) engine; pure, no I/O.
		"regexp.Regexp",                // 🟢 compiled-regex type held in a match slot; pure type, no I/O.
		"slices.Clone",                 // 🟢 copies the $MFT segment list before sorting so the caller's order is untouched; pure, no I/O.
		"slices.SortFunc",              // 🟢 orders $MFT $DATA segments by VCN before merging them; pure function, no I/O.
		"sort.Slice",                   // 🟢 orders immediate children, top-N files, and find blocks by size/name; pure function, no I/O.
		"sort.SliceStable",             // 🟢 stably orders buckets and tree children by size then name; pure function, no I/O.
		"strings.HasSuffix",            // 🟢 checks for a trailing path separator on the target; pure function, no I/O.
		"strings.LastIndexByte",        // 🟢 finds the final ASCII extension separator after a non-ASCII UTF-16 decode; pure function, no I/O.
		"strings.Split",                // 🟢 splits a comma-separated --find-ext value; pure function, no I/O.
		"strings.ToLower",              // 🟢 case-folds child names for stable sort and normalises extensions; pure function, no I/O.
		"strings.Trim",                 // 🟢 identifies dotfile basenames by removing literal '.' delimiters; pure function, no I/O.
		"strings.ToUpper",              // 🟢 upcases the drive letter so paths differing only in drive case resolve identically; pure function, no I/O.
		"strings.TrimPrefix",           // 🟢 strips a leading dot from a normalised extension; pure function, no I/O.
		"strings.TrimSpace",            // 🟢 trims whitespace from split extension tokens; pure function, no I/O.
		"strings.TrimSuffix",           // 🟢 strips a trailing backslash when building enumeration patterns; pure function, no I/O.
		"time.Duration",                // 🟢 pass/wall timing type in Result; pure type, no I/O.
		"time.Now",                     // 🟠 captures scan-phase start times for diagnostics; read-only, no side effects.
		"time.Since",                   // 🟠 computes elapsed pass/wall durations; read-only, no side effects.
		"time.Time",                    // 🟢 created/modified timestamp type on FileEntry; pure data type, no I/O.
		"time.Unix",                    // 🟢 builds a Time from a displayed file's FILETIME nanoseconds during path resolution; pure constructor, no I/O.
		"unsafe.Pointer",               // 🔴 passes the NTFS-volume-data struct to DeviceIoControl and the file-ID descriptor / path buffer to the kernel32 DLL calls via the syscall ABI. No pointer arithmetic; disk bytes are parsed with encoding/binary.
		"unsafe.Sizeof",                // 🔴 computes the byte size of the volume-data and file-ID-descriptor structs passed to DeviceIoControl / OpenFileById; compile-time constant.
		"golang.org/x/sys/windows.ByHandleFileInformation",      // 🟢 struct receiving GetFileInformationByHandle output (volume-internal file index); pure data type.
		"golang.org/x/sys/windows.CloseHandle",                  // 🟠 closes the volume / per-file handles after reads; no data read or exec capability.
		"golang.org/x/sys/windows.CreateFile",                   // 🔴 opens the raw volume device \\.\<drive>: (GENERIC_READ) and per-path metadata handles; read-only raw-device access, requires Administrator. Deliberately bypasses AllowedPaths (see AGENTS.md). TestNTFSMFTRawVolumeCapabilitiesArePinned pins every direct call site plus its access mask and flags.
		"golang.org/x/sys/windows.DeviceIoControl",              // 🔴 issues FSCTL_GET_NTFS_VOLUME_DATA to read the volume geometry; read-only IOCTL, no write capability. TestNTFSMFTRawVolumeCapabilitiesArePinned pins every direct call site and the sole permitted control code.
		"golang.org/x/sys/windows.ERROR_NO_MORE_FILES",          // 🟢 sentinel ending FindNextFile child enumeration; pure constant.
		"golang.org/x/sys/windows.FILE_ATTRIBUTE_DIRECTORY",     // 🟢 flag identifying directory entries during child enumeration; pure constant.
		"golang.org/x/sys/windows.FILE_ATTRIBUTE_REPARSE_POINT", // 🟢 flag marking junctions / symlinks / mount points; pure constant.
		"golang.org/x/sys/windows.FILE_FLAG_BACKUP_SEMANTICS",   // 🟢 CreateFile flag required to open directories by path; pure constant.
		"golang.org/x/sys/windows.FILE_FLAG_OPEN_REPARSE_POINT", // 🟢 CreateFile flag to resolve a reparse point's own MFT idx rather than its target; pure constant.
		"golang.org/x/sys/windows.FILE_READ_ATTRIBUTES",         // 🟢 access mask for OpenFileById metadata-only opens; pure constant.
		"golang.org/x/sys/windows.FILE_SHARE_DELETE",            // 🟢 share mode allowing concurrent delete while scanning; pure constant.
		"golang.org/x/sys/windows.FILE_SHARE_READ",              // 🟢 share mode allowing concurrent reads while scanning; pure constant.
		"golang.org/x/sys/windows.FILE_SHARE_WRITE",             // 🟢 share mode allowing concurrent writes while scanning; pure constant.
		"golang.org/x/sys/windows.FindClose",                    // 🟠 closes a directory-enumeration handle; no data read or exec capability.
		"golang.org/x/sys/windows.FindFirstFile",                // 🟠 begins immediate-child directory enumeration; read-only, no exec capability.
		"golang.org/x/sys/windows.FindNextFile",                 // 🟠 advances immediate-child directory enumeration; read-only, no exec capability.
		"golang.org/x/sys/windows.GENERIC_READ",                 // 🟢 read-only access mask for the volume / path opens; pure constant.
		"golang.org/x/sys/windows.GetFinalPathNameByHandle",     // 🟠 resolves a displayed file's path from an open handle; read-only, no content I/O.
		"golang.org/x/sys/windows.GetFileInformationByHandle",   // 🟠 reads a file's volume-internal identity (MFT index); read-only metadata, no I/O of content.
		"golang.org/x/sys/windows.Handle",                       // 🟢 opaque volume/file handle type; pure type.
		"golang.org/x/sys/windows.InvalidHandle",                // 🟢 sentinel for a failed OpenFileById result; pure constant.
		"golang.org/x/sys/windows.NewLazySystemDLL",             // 🔴 loads kernel32.dll for OpenFileById, which x/sys does not wrap; read-only OS loader call. TestInternalDLLProcsArePinned pins the direct DLL/procedure literals.
		"golang.org/x/sys/windows.OPEN_EXISTING",                // 🟢 CreateFile disposition (open, never create); pure constant.
		"golang.org/x/sys/windows.Overlapped",                   // 🟢 struct carrying the explicit read offset for raw MFT ReadFile calls; pure data type.
		"golang.org/x/sys/windows.ReadFile",                     // 🟠 reads raw MFT bytes from the volume handle at an explicit offset; read-only, no write capability.
		"golang.org/x/sys/windows.UTF16PtrFromString",           // 🟢 converts a Go path to a NUL-terminated UTF-16 pointer for the Windows API; pure function, no I/O.
		"golang.org/x/sys/windows.UTF16ToString",                // 🟢 converts a UTF-16 name/path buffer back to a Go string; pure function, no I/O.
		"golang.org/x/sys/windows.Win32finddata",                // 🟢 struct receiving FindFirstFile / FindNextFile entries; pure data type.
	},
}

// dllProcSet pins the DLL and procedure names a package may load dynamically.
type dllProcSet struct {
	DLLs  []string // NewLazySystemDLL / NewLazyDLL arguments
	Procs []string // NewProc arguments
}

// internalPerPackageDLLProcs pins the DLL and procedure names each
// builtins/internal/<package> may resolve via NewLazySystemDLL/NewLazyDLL/
// NewProc. The symbol allowlist can't see these string arguments (or the
// NewProc method), so TestInternalDLLProcsArePinned enforces them here.
var internalPerPackageDLLProcs = map[string]dllProcSet{
	"ntfsmft": {
		DLLs:  []string{"kernel32.dll"},
		Procs: []string{"OpenFileById"},
	},
	"procinfo": {
		DLLs:  []string{"kernel32.dll"},
		Procs: []string{"GlobalMemoryStatusEx"},
	},
}

// internalAllowedSymbols lists every "importpath.Symbol" permitted in
// builtins/internal/ helper packages. Each entry must be in
// "importpath.Symbol" form with a comment explaining safety.
// This is the global ceiling; each package's per-package allowlist is in
// internalPerPackageSymbols above.
//
// unsafe.Pointer is permitted here for a small, enumerated set of call
// sites that pass a buffer's address into a raw syscall/DLL ABI: winnet
// (GetExtendedTcpTable/GetExtendedUdpTable via iphlpapi.dll), winpoll
// (PeekNamedPipe via kernel32.dll), procmaps (the Darwin
// proc_pidinfo/PROC_PIDREGIONINFO trap), and procinfo (Darwin's fixed-size
// PROC_PIDTASKINFO reply plus Windows' at-most-32-MiB
// SYSTEM_PROCESS_INFORMATION snapshot and fixed memory-status structure).
// There is no unsafe pointer arithmetic: procinfo advances with checked Go
// slice offsets before converting an address, and all other buffer parsing
// uses encoding/binary after the call returns.
var internalAllowedSymbols = []string{
	"bufio.ErrTooLong", // 🟢 diskstats: sentinel error for scanner buffer overflow; pure constant.
	"bufio.NewScanner", // 🟢 procinfo/diskstats: line-by-line reading of /proc files; no write capability.
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath.Default", // 🟢 procinfo/procnet: canonical /proc filesystem root path constant; pure constant, no I/O.
	"bytes.NewReader",                            // 🟢 procinfo: wraps a byte slice as an in-memory io.Reader; no I/O side effects.
	"context.Context",                            // 🟢 procinfo: deadline/cancellation interface; no side effects.
	"encoding/binary.BigEndian",                  // 🟢 winnet: reads big-endian IPv6 group values from DLL buffer; pure value, no I/O.
	"encoding/binary.LittleEndian",               // 🟢 winnet/procinfo: reads fixed-layout little-endian kernel/DLL buffer fields; pure value, no I/O.
	"errors.Is",                                  // 🟢 procinfo: checks whether an error in a chain matches a target; pure function, no I/O.
	"errors.New",                                 // 🟢 creates a sentinel error; pure function, no I/O.
	"github.com/spf13/pflag.ContinueOnError",     // 🟢 flagparser: pflag parsing-mode constant; pure constant, no I/O.
	"github.com/spf13/pflag.FlagSet",             // 🟢 flagparser: pflag FlagSet type used to trial-parse argv prefixes; pure type, no I/O.
	"github.com/spf13/pflag.NewFlagSet",          // 🟢 flagparser: constructs a throw-away FlagSet for trial-parsing; pure constructor, no I/O.
	"io.Discard",                                 // 🟢 flagparser: silences trial.SetOutput so trial-parse failures don't leak to stderr; pure no-op writer.
	"io.LimitReader",                             // 🟢 procinfo/procmaps/procsyskernel: caps kernel pseudo-file reads before buffering; pure wrapper.
	"io.ReadAll",                                 // 🟠 procinfo/procmaps/procsyskernel: buffers only readers already capped by io.LimitReader.
	"math.MaxUint32",                             // 🟢 procmaps: upper bound for a valid PID before the uint32(pid) OpenProcess cast; pure constant.
	"math/bits.Div64",                            // 🟢 procinfo (darwin): overflow-safe division for CPU tick conversion; pure arithmetic.
	"math/bits.Mul64",                            // 🟢 procinfo (darwin): overflow-safe multiplication for CPU tick conversion; pure arithmetic.
	"math/bits.OnesCount32",                      // 🟢 procnet: counts set bits in a uint32 (popcount for prefix length); pure function, no I/O.
	"math/bits.ReverseBytes32",                   // 🟢 procnet: byte-swaps a uint32 to convert little-endian /proc mask to network byte order for CIDR validation; pure function, no I/O.
	"fmt.Errorf",                                 // 🟢 error formatting; pure function, no I/O.
	"os.ErrNotExist",                             // 🟢 procinfo: sentinel error value indicating a file or directory does not exist; read-only constant, no I/O.
	"fmt.Sprintf",                                // 🟢 string formatting; pure function, no I/O.
	"io.Reader",                                  // 🟢 diskstats: interface type used to feed parseMountInfo from arbitrary readers; pure type, no I/O.
	"os.Getpid",                                  // 🟠 procinfo: returns the current process ID; read-only, no side effects.
	"os.ModeCharDevice",                          // 🟢 procsyskernel: file mode constant for char device detection; pure constant.
	"os.O_RDONLY",                                // 🟢 procsyskernel: read-only open flag; pure constant.
	"os.Open",                                    // 🟠 procinfo: opens a file read-only; needed to stream /proc/stat line-by-line.
	"os.OpenFile",                                // 🟠 procsyskernel: opens kernel pseudo-files with O_NONBLOCK; bypasses AllowedPaths by design.
	"os.ReadDir",                                 // 🟠 procinfo/procfd: reads a directory listing; needed to enumerate /proc entries.
	"os.ReadFile",                                // 🟠 procinfo/procfd: reads a whole file; needed to read /proc/[pid]/{stat,status}.
	"os.Readlink",                                // 🟠 procfd: resolves an fd/cwd/root/exe magic symlink's kernel-reported target; read-only, no write capability.
	"os.Stat",                                    // 🟠 procinfo: validates that the proc path exists before enumeration; read-only metadata, no write capability.
	"path/filepath.Base",                         // 🟢 procsyskernel: returns the last element of a path; validates name is a plain basename.
	"path/filepath.Clean",                        // 🟢 procnetroute/procnetsocket: normalises procPath before ".." safety check; pure function, no I/O.
	"path/filepath.Join",                         // 🟢 procinfo/procfd: joins path elements to construct /proc/<pid>/... paths; pure function, no I/O.
	"runtime.KeepAlive",                          // 🟠 procinfo (windows): pins Go buffers across read-only syscall/DLL ABI calls; no I/O itself.
	"slices.Sort",                                // 🟢 procfd: sorts the scanned PID list ascending; pure function, no I/O.
	"slices.SortFunc",                            // 🟢 procfd: sorts numeric fds ascending by their parsed int value; pure function, no I/O.
	"strconv.QuoteToGraphic",                     // 🟢 flagparser: escapes control, format, and malformed bytes in option diagnostics; pure conversion.
	"strconv.Atoi",                               // 🟢 string-to-int conversion; pure function, no I/O.
	"strconv.Itoa",                               // 🟢 procinfo/procfd: int-to-string conversion for PID directory names; pure function, no I/O.
	"strconv.ParseFloat",                         // 🟢 sysinfo/vmstat (linux): parses uptime and load-average floats from /proc; pure function, no I/O.
	"strconv.ParseInt",                           // 🟢 procinfo: string to int64 with base/bit-size; pure function, no I/O.
	"strconv.FormatInt",                          // 🟢 procfd: formats a file size as a decimal string; pure function, no I/O.
	"strconv.FormatUint",                         // 🟢 procnetsocket/procfd: uint-to-string conversion for port/inode formatting; pure function, no I/O.
	"strconv.ParseUint",                          // 🟢 procinfo/procnetroute/procnetsocket: parses unsigned procfs counters and fields; pure function, no I/O.
	"strings.Builder",                            // 🟢 procnetsocket/diskstats: efficient string concatenation; pure in-memory buffer, no I/O.
	"strings.Contains",                           // 🟢 procnetroute/diskstats: substring check; pure function, no I/O.
	"strings.ContainsRune",                       // 🟢 diskstats: fast-path check for backslash before unescape; pure function, no I/O.
	"strings.Cut",                                // 🟢 diskstats: splits a string at the first separator; pure function, no I/O.
	"strings.Fields",                             // 🟢 procinfo/procnetroute/procnetsocket/diskstats: splits a string on whitespace; pure function, no I/O.
	"strings.Join",                               // 🟢 procnetsocket: reconstructs space-containing Unix socket paths from Fields tokens; pure function, no I/O.
	"strings.Split",                              // 🟢 procnetsocket: splits address:port fields on ":"; pure function, no I/O.
	"strings.ToUpper",                            // 🟢 procnetsocket: normalises hex state field to uppercase for map lookup; pure function, no I/O.
	"strings.CutPrefix",                          // 🟢 flagparser: trims known pflag error prefixes before rewriting; pure function, no I/O.
	"strings.CutSuffix",                          // 🟢 meminfo/procfd: strips the trailing " kB" unit / " (deleted)" marker before further parsing; pure function, no I/O.
	"strings.HasPrefix",                          // 🟢 procinfo/diskstats: checks string prefix; pure function, no I/O.
	"strings.HasSuffix",                          // 🟢 flagparser: matches pflag error suffixes (e.g. "flag does not allow an argument"); pure function, no I/O.
	"strings.Index",                              // 🟢 procinfo: finds first occurrence of a substring; pure function, no I/O.
	"strings.IndexByte",                          // 🟢 procmaps: finds the '-' separator in a /proc/pid/maps address range; pure function, no I/O.
	"strings.LastIndex",                          // 🟢 procinfo: finds last occurrence of a substring; pure function, no I/O.
	"strings.TrimRight",                          // 🟢 procinfo: trims trailing characters; pure function, no I/O.
	"strings.TrimSpace",                          // 🟢 procinfo: removes leading/trailing whitespace; pure function, no I/O.
	"syscall.EPERM",                              // 🟢 procmaps (darwin): sentinel errno distinguishing "caller lacks privilege" from "end of region walk"; pure constant.
	"syscall.Errno",                              // 🟢 winnet: wraps DLL return code as an error type; pure type, no I/O.
	"syscall.Getsid",                             // 🟠 procinfo: returns the session ID of a process; read-only syscall, no write/exec.
	"syscall.O_NONBLOCK",                         // 🟢 procsyskernel: non-blocking open flag to prevent FIFO hang; pure constant.
	"syscall.MustLoadDLL",                        // 🔴 winnet: loads iphlpapi.dll once at program init; read-only OS loader call.
	"syscall.Proc",                               // 🟢 winnet: DLL procedure handle type used in function signature; pure type, no I/O.
	"syscall.SYS_PROC_INFO",                      // 🟢 procmaps/procinfo (darwin): raw trap number for read-only proc_pidinfo queries; pure constant.
	"syscall.Syscall6",                           // 🟠 procmaps/procinfo (darwin): invokes fixed read-only proc_pidinfo region/task queries; no exec or write capability.
	"time.Duration",                              // 🟢 procinfo: duration type for CPU and elapsed metrics; pure integer alias, no I/O.
	"time.Now",                                   // 🟠 procinfo: returns the current wall-clock time; read-only, no side effects.
	"time.ParseDuration",                         // 🟢 procinfo: parses Linux uptime text into a duration; pure function, no I/O.
	"time.Second",                                // 🟢 procinfo: duration constant for CPU tick/time conversions; no side effects.
	"time.Time",                                  // 🟢 procinfo: process start-time value type; pure data, no side effects.
	"time.Unix",                                  // 🟢 procinfo: constructs a Time from Unix seconds; pure function, no I/O.
	"unicode.IsGraphic",                          // 🟢 procinfo: reports whether a process-name rune is printable; pure function, no I/O.
	"unicode/utf8.DecodeRuneInString",            // 🟢 procinfo: decodes process names without splitting UTF-8 sequences; pure function, no I/O.
	"unicode/utf8.RuneError",                     // 🟢 procinfo: replacement rune constant used to detect malformed UTF-8; no side effects.
	"unsafe.Alignof",                             // 🟢 procinfo (windows): reports kernel ABI record alignment; compile-time layout query.
	"unsafe.Pointer",                             // 🔴 winnet/procmaps/procinfo: passes fixed or bounded buffers through read-only syscall/DLL ABIs; see the audit note above.
	"unsafe.Sizeof",                              // 🟢 procinfo (windows): reports fixed ABI structure sizes; compile-time layout query.
	"golang.org/x/sys/unix.ByteSliceToString",    // 🟢 diskstats (darwin): converts a NUL-terminated kernel byte buffer to a Go string; pure function, no I/O.
	"golang.org/x/sys/unix.Getfsstat",            // 🟠 diskstats (darwin): read-only enumeration of mounted filesystems via getfsstat(2); no exec or write capability.
	"golang.org/x/sys/unix.KinfoProc",            // 🟢 procinfo (darwin): struct type carrying per-process kinfo_proc data from sysctl; read-only data, no exec capability.
	"golang.org/x/sys/unix.Major",                // 🟢 procfd (linux): extracts the major device number from a raw dev_t; pure function, no I/O.
	"golang.org/x/sys/unix.Minor",                // 🟢 procfd (linux): extracts the minor device number from a raw dev_t; pure function, no I/O.
	"golang.org/x/sys/unix.MNT_LOCAL",            // 🟢 diskstats (darwin): flag constant indicating a local-only filesystem; pure constant.
	"golang.org/x/sys/unix.MNT_NOWAIT",           // 🟢 diskstats (darwin): flag constant: do not block on remote FS for getfsstat; pure constant.
	"golang.org/x/sys/unix.S_IFBLK",              // 🟢 procfd (linux): file-type bitmask constant for block devices; pure constant.
	"golang.org/x/sys/unix.S_IFCHR",              // 🟢 procfd (linux): file-type bitmask constant for character devices; pure constant.
	"golang.org/x/sys/unix.S_IFDIR",              // 🟢 procfd (linux): file-type bitmask constant for directories; pure constant.
	"golang.org/x/sys/unix.S_IFIFO",              // 🟢 procfd (linux): file-type bitmask constant for FIFOs; pure constant.
	"golang.org/x/sys/unix.S_IFLNK",              // 🟢 procfd (linux): file-type bitmask constant for symlinks; pure constant.
	"golang.org/x/sys/unix.S_IFMT",               // 🟢 procfd (linux): file-type bitmask mask constant; pure constant.
	"golang.org/x/sys/unix.S_IFREG",              // 🟢 procfd (linux): file-type bitmask constant for regular files; pure constant.
	"golang.org/x/sys/unix.S_IFSOCK",             // 🟢 procfd (linux): file-type bitmask constant for sockets; pure constant.
	"golang.org/x/sys/unix.Stat",                 // 🟠 procfd (linux): stats an fd's magic symlink path to get Type/Device/Size/Node, including for deleted-but-open files; read-only, no write capability.
	"golang.org/x/sys/unix.Stat_t",               // 🟢 procfd (linux): struct type carrying stat(2) data; pure data type.
	"golang.org/x/sys/unix.Statfs",               // 🟠 diskstats (linux): read-only filesystem usage syscall; no exec or write capability.
	"golang.org/x/sys/unix.Statfs_t",             // 🟢 diskstats: struct type carrying filesystem usage data from statfs/getfsstat; pure data type.
	"golang.org/x/sys/unix.SysctlKinfoProc",      // 🟠 procinfo (darwin): reads a single process's kinfo_proc via kern.proc.pid sysctl; read-only, no exec or write capability.
	"golang.org/x/sys/unix.SysctlKinfoProcSlice", // 🟠 procinfo (darwin): reads all processes' kinfo_proc via kern.proc.all sysctl; read-only, no exec or write capability.
	"golang.org/x/sys/unix.SysctlRaw",            // 🟠 sysinfo/vmstat (darwin): reads kern.boottime, vm.swapusage, and vm.loadavg via read-only sysctl; no exec or write capability.
	"golang.org/x/sys/windows.CloseHandle",       // 🟠 procinfo (windows): closes a process-snapshot handle after enumeration; no data read or exec capability.
	"io.EOF",                                     // 🟢 vmstat: sentinel error value returned by bufio.Scanner at end of file; pure constant.
	"math.IsInf",                                 // 🟢 vmstat: rejects infinite load-average/uptime values; pure function, no I/O.
	"math.IsNaN",                                 // 🟢 vmstat: rejects NaN load-average/uptime values; pure function, no I/O.
	"math.MaxUint64",                             // 🟢 vmstat: integer constant; bounds the /proc/meminfo KiB-to-bytes conversion against overflow; no side effects.
	"os.Getpagesize",                             // 🟢 procinfo/vmstat: returns the host's memory page size; read-only, no I/O.
	"golang.org/x/sys/unix.Getpagesize",          // 🟢 vmstat (darwin): returns the host's memory page size; read-only, no I/O.
	"golang.org/x/sys/unix.SysctlUint64",         // 🟠 procinfo/vmstat (darwin): reads hw.memsize/hw.tbfrequency counters; read-only sysctl.
	"golang.org/x/sys/windows.CreateToolhelp32Snapshot",      // 🟠 procinfo (windows): creates a read-only snapshot of the process table; no exec or write capability.
	"golang.org/x/sys/windows.ERROR_BROKEN_PIPE",             // 🟢 winpoll (windows): sentinel error from PeekNamedPipe when the writer end has closed — used to recognize EOF-ready pipes; pure constant.
	"golang.org/x/sys/windows.ERROR_INVALID_PARAMETER",       // 🟢 procmaps (windows): sentinel error from OpenProcess when the PID does not name a running process; pure constant.
	"golang.org/x/sys/windows.ERROR_NO_MORE_FILES",           // 🟢 procinfo (windows): sentinel error indicating end of process enumeration; pure constant.
	"golang.org/x/sys/windows.FILE_TYPE_CHAR",                // 🟢 winpoll (windows): GetFileType result for console/character devices; pure constant.
	"golang.org/x/sys/windows.FILE_TYPE_DISK",                // 🟢 winpoll (windows): GetFileType result for regular files; pure constant.
	"golang.org/x/sys/windows.FILE_TYPE_PIPE",                // 🟢 winpoll (windows): GetFileType result for anonymous and named pipes; pure constant.
	"golang.org/x/sys/windows.FILE_TYPE_REMOTE",              // 🟢 winpoll (windows): GetFileType modifier bit for remote-mounted volumes; pure constant.
	"golang.org/x/sys/windows.GetFileType",                   // 🟠 winpoll (windows): returns the type (disk/pipe/char/remote/unknown) of an open handle; read-only metadata, no I/O.
	"golang.org/x/sys/windows.GetModuleFileNameEx",           // 🟠 procmaps (windows): reads a process's main module file path; read-only, no exec capability.
	"golang.org/x/sys/windows.GetNumberOfConsoleInputEvents", // 🟠 winpoll (windows): reports the count of queued console input events without consuming them; read-only inspection.
	"golang.org/x/sys/windows.Handle",                        // 🟢 winpoll (windows): opaque file/handle type used to call PeekNamedPipe and GetFileType; pure type.
	"golang.org/x/sys/windows.MEM_COMMIT",                    // 🟢 procmaps (windows): MEMORY_BASIC_INFORMATION.State value selecting committed regions; pure constant.
	"golang.org/x/sys/windows.MemoryBasicInformation",        // 🟢 procmaps (windows): struct type carrying VirtualQueryEx region data; pure data type, no I/O.
	"golang.org/x/sys/windows.OpenProcess",                   // 🟠 procmaps/procinfo (windows): opens a process with query/VM-read or query-limited rights only; no write or exec capability.
	"golang.org/x/sys/windows.PAGE_EXECUTE",                  // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_EXECUTE_READ",             // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_EXECUTE_READWRITE",        // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_EXECUTE_WRITECOPY",        // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_GUARD",                    // 🟢 procmaps (windows): PAGE_* protection modifier bit masked off before mode classification; pure constant.
	"golang.org/x/sys/windows.PAGE_NOCACHE",                  // 🟢 procmaps (windows): PAGE_* protection modifier bit masked off before mode classification; pure constant.
	"golang.org/x/sys/windows.PAGE_READONLY",                 // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_READWRITE",                // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PAGE_WRITECOMBINE",             // 🟢 procmaps (windows): PAGE_* protection modifier bit masked off before mode classification; pure constant.
	"golang.org/x/sys/windows.PAGE_WRITECOPY",                // 🟢 procmaps (windows): PAGE_* protection constant; pure constant.
	"golang.org/x/sys/windows.PROCESS_QUERY_INFORMATION",     // 🟢 procmaps (windows): OpenProcess access-right constant; pure constant.
	"golang.org/x/sys/windows.PROCESS_VM_READ",               // 🟢 procmaps (windows): OpenProcess access-right constant; pure constant.
	"golang.org/x/sys/windows.Process32First",                // 🟠 procinfo (windows): reads the first entry from a process snapshot; read-only, no exec capability.
	"golang.org/x/sys/windows.Process32Next",                 // 🟠 procinfo (windows): advances to the next entry in a process snapshot; read-only, no exec capability.
	"golang.org/x/sys/windows.ProcessEntry32",                // 🟢 procinfo (windows): struct type holding process snapshot entry data; pure data type, no I/O.
	"golang.org/x/sys/windows.TH32CS_SNAPPROCESS",            // 🟢 procinfo (windows): flag constant selecting process entries for CreateToolhelp32Snapshot; pure constant.
	"golang.org/x/sys/windows.UTF16ToString",                 // 🟢 procinfo (windows): converts a null-terminated UTF-16 slice to a Go string; pure function, no I/O.
	"golang.org/x/sys/windows.VirtualQueryEx",                // 🟠 procmaps (windows): read-only enumeration of another process's virtual memory regions; no exec or write capability.

	"golang.org/x/sys/windows.Filetime",                          // 🟢 procinfo (windows): fixed-layout process timestamp data; pure data type.
	"golang.org/x/sys/windows.GetProcessTimes",                   // 🟠 procinfo (windows): reads creation/CPU times through a query-only handle.
	"golang.org/x/sys/windows.NewLazySystemDLL",                  // 🔴 procinfo (windows): resolves fixed kernel32.dll for read-only GlobalMemoryStatusEx.
	"golang.org/x/sys/windows.NtQuerySystemInformation",          // 🔴 procinfo (windows): reads a process snapshot into an at-most-32-MiB buffer; no write/exec capability.
	"golang.org/x/sys/windows.PROCESS_QUERY_LIMITED_INFORMATION", // 🟢 procinfo (windows): query-only OpenProcess access-right constant; pure constant.
	"golang.org/x/sys/windows.STATUS_INFO_LENGTH_MISMATCH",       // 🟢 procinfo (windows): retry sentinel for an undersized bounded snapshot buffer.
	"golang.org/x/sys/windows.SYSTEM_PROCESS_INFORMATION",        // 🔴 procinfo (windows): kernel ABI record parsed only after alignment and bounds checks.
	"golang.org/x/sys/windows.SystemProcessInformation",          // 🟢 procinfo (windows): query class selecting read-only process information; pure constant.

	"bytes.Equal",           // 🟢 ntfsmft: compares decoded extension bytes; pure function, no I/O.
	"cmp.Compare",           // 🟢 ntfsmft: orders $MFT $DATA segments by VCN; pure function, no I/O.
	"container/heap.Fix",    // 🟢 ntfsmft: re-establishes a min-heap invariant; pure in-memory.
	"container/heap.Push",   // 🟢 ntfsmft: pushes onto a top-N min-heap; pure in-memory.
	"math.MaxInt64",         // 🟢 ntfsmft: rejects raw NTFS size fields whose high bit is set (would wrap negative); pure constant, no I/O.
	"path/filepath.Clean",   // 🟢 ntfsmft: normalises already-absolute target/exclude paths; pure function, no I/O.
	"path/filepath.IsAbs",   // 🟢 ntfsmft: rejects relative target/exclude paths so the host process cwd is never consulted; pure function, no I/O.
	"path/filepath.Match",   // 🟢 ntfsmft: evaluates a --find-glob pattern against a basename; pure function, no I/O.
	"regexp.Compile",        // 🟢 ntfsmft: compiles a --find-regex pattern with the RE2 engine; pure, no I/O.
	"regexp.Regexp",         // 🟢 ntfsmft: compiled-regex type held in a match slot; pure type, no I/O.
	"slices.Clone",          // 🟢 ntfsmft: copies the $MFT segment list before sorting, leaving the caller's order intact; pure, no I/O.
	"slices.SortFunc",       // 🟢 ntfsmft: orders $MFT $DATA segments by VCN before merging; pure function, no I/O.
	"sort.Slice",            // 🟢 ntfsmft: orders children/top-N/find results by size/name; pure function, no I/O.
	"sort.SliceStable",      // 🟢 ntfsmft: stably orders buckets/tree children; pure function, no I/O.
	"strings.LastIndexByte", // 🟢 ntfsmft: finds the final ASCII extension separator after a non-ASCII UTF-16 decode; pure function, no I/O.
	"strings.ToLower",       // 🟢 ntfsmft: case-folds names and normalises extensions; pure function, no I/O.
	"strings.Trim",          // 🟢 ntfsmft: removes literal '.' delimiters to identify dotfile basenames; pure function, no I/O.
	"strings.TrimPrefix",    // 🟢 ntfsmft: strips a leading dot from an extension; pure function, no I/O.
	"strings.TrimSuffix",    // 🟢 ntfsmft: strips a trailing backslash from enumeration patterns; pure function, no I/O.
	"time.Duration",         // 🟢 ntfsmft: pass/wall timing type; pure type, no I/O.
	"time.Since",            // 🟠 ntfsmft: computes elapsed pass/wall durations; read-only, no side effects.
	"time.Time",             // 🟢 ntfsmft: created/modified timestamp type on FileEntry; pure data type, no I/O.
	"time.Unix",             // 🟢 ntfsmft: builds a Time from a displayed file's FILETIME nanoseconds; pure constructor, no I/O.
	"unsafe.Sizeof",         // 🔴 ntfsmft: byte size of structs passed to DeviceIoControl / OpenFileById; compile-time constant.
	"golang.org/x/sys/windows.ByHandleFileInformation",      // 🟢 ntfsmft (windows): struct receiving GetFileInformationByHandle output; pure data type.
	"golang.org/x/sys/windows.CreateFile",                   // 🔴 ntfsmft (windows): opens the raw volume device \\.\<drive>: (GENERIC_READ) and per-path metadata handles; read-only raw-device access, requires Administrator. Bypasses AllowedPaths by design (see AGENTS.md). TestNTFSMFTRawVolumeCapabilitiesArePinned pins every direct call site plus its access mask and flags.
	"golang.org/x/sys/windows.DeviceIoControl",              // 🔴 ntfsmft (windows): FSCTL_GET_NTFS_VOLUME_DATA read-only IOCTL for volume geometry; no write capability. TestNTFSMFTRawVolumeCapabilitiesArePinned pins every direct call site and the sole permitted control code.
	"golang.org/x/sys/windows.FILE_ATTRIBUTE_DIRECTORY",     // 🟢 ntfsmft (windows): directory-entry flag; pure constant.
	"golang.org/x/sys/windows.FILE_ATTRIBUTE_REPARSE_POINT", // 🟢 ntfsmft (windows): junction/symlink/mount-point flag; pure constant.
	"golang.org/x/sys/windows.FILE_FLAG_BACKUP_SEMANTICS",   // 🟢 ntfsmft (windows): CreateFile flag to open directories by path; pure constant.
	"golang.org/x/sys/windows.FILE_FLAG_OPEN_REPARSE_POINT", // 🟢 ntfsmft (windows): CreateFile flag to resolve a reparse point's own MFT idx; pure constant.
	"golang.org/x/sys/windows.FILE_READ_ATTRIBUTES",         // 🟢 ntfsmft (windows): access mask for OpenFileById metadata-only opens; pure constant.
	"golang.org/x/sys/windows.FILE_SHARE_DELETE",            // 🟢 ntfsmft (windows): share mode allowing concurrent delete; pure constant.
	"golang.org/x/sys/windows.FILE_SHARE_READ",              // 🟢 ntfsmft (windows): share mode allowing concurrent reads; pure constant.
	"golang.org/x/sys/windows.FILE_SHARE_WRITE",             // 🟢 ntfsmft (windows): share mode allowing concurrent writes; pure constant.
	"golang.org/x/sys/windows.FindClose",                    // 🟠 ntfsmft (windows): closes a directory-enumeration handle; no data read or exec capability.
	"golang.org/x/sys/windows.FindFirstFile",                // 🟠 ntfsmft (windows): begins immediate-child directory enumeration; read-only.
	"golang.org/x/sys/windows.FindNextFile",                 // 🟠 ntfsmft (windows): advances immediate-child directory enumeration; read-only.
	"golang.org/x/sys/windows.GENERIC_READ",                 // 🟢 ntfsmft (windows): read-only access mask; pure constant.
	"golang.org/x/sys/windows.GetFinalPathNameByHandle",     // 🟠 ntfsmft (windows): resolves a displayed file's path from an open handle; read-only, no content I/O.
	"golang.org/x/sys/windows.GetFileInformationByHandle",   // 🟠 ntfsmft (windows): reads a file's volume-internal identity (MFT index); read-only metadata.
	"golang.org/x/sys/windows.InvalidHandle",                // 🟢 ntfsmft (windows): sentinel for a failed OpenFileById result; pure constant.
	"golang.org/x/sys/windows.NewLazySystemDLL",             // 🔴 ntfsmft (windows): loads kernel32.dll for OpenFileById, which x/sys does not wrap; read-only OS loader call. TestInternalDLLProcsArePinned pins the direct DLL/procedure literals.
	"golang.org/x/sys/windows.OPEN_EXISTING",                // 🟢 ntfsmft (windows): CreateFile disposition (open, never create); pure constant.
	"golang.org/x/sys/windows.Overlapped",                   // 🟢 ntfsmft (windows): struct carrying the explicit read offset for raw MFT reads; pure data type.
	"golang.org/x/sys/windows.ReadFile",                     // 🟠 ntfsmft (windows): reads raw MFT bytes at an explicit offset; read-only, no write capability.
	"golang.org/x/sys/windows.UTF16PtrFromString",           // 🟢 ntfsmft (windows): converts a Go path to a NUL-terminated UTF-16 pointer; pure function, no I/O.
	"golang.org/x/sys/windows.Win32finddata",                // 🟢 ntfsmft (windows): struct receiving FindFirstFile / FindNextFile entries; pure data type.
}
