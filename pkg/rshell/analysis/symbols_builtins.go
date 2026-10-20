// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

// builtinAllowedSymbols lists every "importpath.Symbol" that may be used by
// command implementation files in builtins/. Each entry must be in
// "importpath.Symbol" form, where importpath is the full Go import path.
//
// Each symbol must have a comment explaining what it does and why it is safe
// to use inside a sandboxed builtin (e.g. pure function, constant, interface,
// no filesystem/network/exec side effects).
//
// To use a new symbol, add a single line here with its safety justification.
//
// Permanently banned (cannot be added):
//   - reflect  — reflection defeats static safety analysis
//   - unsafe   — bypasses Go's type and memory safety guarantees
//
// All packages not listed here are implicitly banned, including all
// third-party packages and other internal module packages.

// builtinPerCommandSymbols maps each builtin command name (matching its
// subdirectory under builtins/) to the symbols it is allowed to use.
// Every symbol listed here must also appear in builtinAllowedSymbols
// (which acts as the global ceiling).
var builtinPerCommandSymbols = map[string][]string{
	"awk": {
		"bufio.NewScanner",                // 🟢 line-by-line record reading; no write or exec capability.
		"bufio.Scanner",                   // 🟢 scanner type retained for incremental main-input reads; no write or exec capability.
		"bytes.Index",                     // 🟢 finds a byte sequence in a byte slice; pure function, no I/O.
		"context.Background",              // 🟢 supplies a non-canceling context for parser and regex helper entrypoints; no side effects.
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",                       // 🟢 error comparison; pure function, no I/O.
		"errors.New",                      // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",                      // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                     // 🟢 string formatting for awk printf; pure function, no I/O.
		"io.EOF",                          // 🟢 sentinel error value; pure constant.
		"io.NopCloser",                    // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",                   // 🟢 interface type; no side effects.
		"math.IsInf",                      // 🟢 rejects infinite dynamic printf widths and precisions; pure function, no I/O.
		"math.IsNaN",                      // 🟢 IEEE 754 NaN check; pure function, no I/O.
		"math.Mod",                        // 🟢 pure arithmetic modulo for awk % operator; no side effects.
		"math.Pow",                        // 🟢 pure exponentiation for awk ^ and ^= operators; no side effects.
		"math.Trunc",                      // 🟢 pure arithmetic truncation for awk int(); no side effects.
		"os.O_RDONLY",                     // 🟢 read-only file flag constant; cannot open files by itself.
		"regexp.Compile",                  // 🟢 compiles a regular expression; pure function, no I/O. Uses RE2 engine (linear-time, no backtracking).
		"regexp.Regexp",                   // 🟢 compiled regular expression type; no I/O side effects. All matching methods are linear-time (RE2).
		"regexp/syntax.MatchNL",           // 🟢 parser flag enabling record-spanning character matches; pure constant.
		"regexp/syntax.OneLine",           // 🟢 parser flag giving anchors whole-record semantics; pure constant.
		"regexp/syntax.Parse",             // 🟢 parses a regular expression into an in-memory AST; no I/O or side effects.
		"regexp/syntax.POSIX",             // 🟢 parser mode restricting patterns to POSIX ERE syntax; pure constant.
		"slices.SortFunc",                 // 🟢 sorts a slice with a comparison function; pure function, no I/O.
		"strconv.ErrRange",                // 🟢 sentinel error accepted for numeric overflow; pure constant.
		"strconv.FormatFloat",             // 🟢 float-to-string conversion for awk numeric output; pure function.
		"strconv.Itoa",                    // 🟢 int-to-string conversion for bounded dynamic printf dimensions; pure function, no I/O.
		"strconv.ParseFloat",              // 🟢 string-to-float conversion; pure function, no I/O.
		"strings.Builder",                 // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.Compare",                 // 🟢 compares two strings lexicographically; pure function, no I/O.
		"strings.ContainsRune",            // 🟢 checks if a rune is in a string; pure function, no I/O.
		"strings.Cut",                     // 🟢 splits a string around the first separator; pure function, no I/O.
		"strings.HasPrefix",               // 🟢 pure prefix check used to classify regular-expression errors; no I/O.
		"strings.Index",                   // 🟢 substring search for awk index(); pure function, no I/O.
		"strings.Join",                    // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
		"strings.NewReader",               // 🟢 wraps a string as an io.Reader; pure in-memory, no I/O.
		"strings.ReplaceAll",              // 🟢 removes nonnumeric zero-padding flags from awk printf formats; pure function, no I/O.
		"strings.SplitN",                  // 🟢 bounded string splitting; pure function, no I/O.
		"strings.ToLower",                 // 🟢 converts string to lowercase for awk tolower(); pure function, no I/O.
		"strings.ToUpper",                 // 🟢 converts string to uppercase for awk toupper(); pure function, no I/O.
		"strings.Trim",                    // 🟢 removes a fixed set of leading/trailing characters; pure function.
		"sync.Once",                       // 🟢 ensures each record source closes its reader at most once.
		"time.Time",                       // 🟢 time value type used to reset borrowed-stdin deadlines; pure data, no side effects.
		"time.Unix",                       // 🟢 constructs a past deadline used to interrupt a blocked borrowed-stdin read.
		"unicode/utf8.DecodeRuneInString", // 🟢 decodes first UTF-8 rune from a string; pure function, no I/O.
		"unicode/utf8.RuneError",          // 🟢 replacement character returned for invalid UTF-8; constant, no I/O.
		"unicode/utf8.UTFMax",             // 🟢 maximum UTF-8 encoding width used to size the bounded record scanner; pure constant.

		"golang.org/x/sys/unix.EINTR",      // 🟢 interrupted-system-call sentinel used to retry bounded poll waits.
		"golang.org/x/sys/unix.FD_SETSIZE", // 🟢 maximum descriptor supported by Darwin select; pure constant.
		"golang.org/x/sys/unix.FdSet",      // 🟢 descriptor set used for a non-consuming Darwin program-source readiness wait.
		"golang.org/x/sys/unix.F_GETFL",    // 🟢 descriptor-status query constant; pure constant.
		"golang.org/x/sys/unix.FcntlInt",   // 🟠 reads descriptor flags to select a non-mutating cancellation strategy.
		"golang.org/x/sys/unix.O_NONBLOCK", // 🟢 nonblocking descriptor flag constant; pure constant.
		"golang.org/x/sys/unix.POLLHUP",    // 🟢 poll event constant for peer closure; pure constant.
		"golang.org/x/sys/unix.POLLIN",     // 🟢 poll event constant for readable input; pure constant.
		"golang.org/x/sys/unix.Poll",       // 🟠 waits with a bounded timeout for non-consuming program-source readiness.
		"golang.org/x/sys/unix.PollFd",     // 🟢 descriptor data passed to unix.Poll; pure data type.
		"golang.org/x/sys/unix.Select",     // 🟠 waits for Darwin program-source readiness, including drained FIFO EOF.
		"golang.org/x/sys/unix.Timeval",    // 🟢 bounded timeout passed to Darwin select; pure data type.
		"syscall.RawConn",                  // 🟠 pins a caller-owned descriptor while its flags and readiness are inspected.
	},
	"break": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
	},
	"cat": {
		"bufio.NewScanner", // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"context.Context",  // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",        // 🟢 error comparison; pure function, no I/O.
		"io.EOF",           // 🟢 sentinel error value; pure constant.
		"io.NopCloser",     // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",    // 🟢 interface type; no side effects.
		"os.O_RDONLY",      // 🟢 read-only file flag constant; cannot open files by itself.
	},
	"cd": {
		"context.Context",          // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",                // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",                // 🟢 error sentinel comparison; pure function, no I/O.
		"errors.New",               // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",               // 🟢 error formatting; pure function, no I/O.
		"io/fs.ErrPermission",      // 🟢 sentinel error for permission denied; pure constant.
		"io/fs.ModeSymlink",        // 🟢 file mode bit constant for symlinks; pure constant.
		"os.PathError",             // 🟢 error type wrapping path and operation; pure type, no I/O.
		"path/filepath.Clean",      // 🟢 normalizes a path lexically (collapses ".", "..", duplicate separators); pure function, no I/O.
		"path/filepath.Dir",        // 🟢 returns the directory component of a path; pure function, no I/O.
		"path/filepath.FromSlash",  // 🟢 converts '/' to OS separator without otherwise normalising; pure function, no I/O.
		"path/filepath.IsAbs",      // 🟢 reports whether a path is absolute; pure function, no I/O.
		"path/filepath.Join",       // 🟢 lexically joins path components with the OS separator; pure function, no I/O.
		"path/filepath.Separator",  // 🟢 OS path separator constant ('/' or '\\'); pure constant, no I/O.
		"path/filepath.VolumeName", // 🟢 returns the volume prefix of a path (e.g. "C:" on Windows, "" on Unix); pure function, no I/O.
		"strings.HasPrefix",        // 🟢 pure function for prefix matching; no I/O.
		"strings.HasSuffix",        // 🟢 pure function for suffix matching; no I/O.
		"strings.IndexByte",        // 🟢 finds byte in string; pure function, no I/O.
		"strings.TrimPrefix",       // 🟢 removes a leading prefix from a string; pure function, no I/O.
	},
	"continue": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
	},
	"cut": {
		"bufio.NewScanner",  // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"io.NopCloser",      // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",     // 🟢 interface type; no side effects.
		"math.MaxInt32",     // 🟢 integer constant; no side effects.
		"os.O_RDONLY",       // 🟢 read-only file flag constant; cannot open files by itself.
		"slices.SortFunc",   // 🟢 sorts a slice with a comparison function; pure function, no I/O.
		"strconv.Atoi",      // 🟢 string-to-int conversion; pure function, no I/O.
		"strings.IndexByte", // 🟢 finds byte in string; pure function, no I/O.
		"strings.Split",     // 🟢 splits a string by separator into a slice; pure function, no I/O.
	},
	"df": {
		"context.Context",    // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",          // 🟢 error comparison via chain; pure function, no I/O.
		"errors.New",         // 🟢 creates a sentinel error (unitFlag.Set rejects explicit values); pure function, no I/O.
		"fmt.Sprintf",        // 🟢 string formatting; pure function, no I/O.
		"math.Ceil",          // 🟢 ceiling of a float64; pure function, no I/O. Used for GNU-compatible round-up of human-readable sizes.
		"strconv.FormatUint", // 🟢 uint-to-string conversion; pure function, no I/O.
		"strings.Builder",    // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.Repeat",     // 🟢 returns a string of n repetitions; pure function, no I/O.
		// Note: builtins/internal/diskstats symbols are exempt from this
		// allowlist (internal packages are not checked by the
		// builtinAllowedSymbols test).
	},
	"echo": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"strings.Builder", // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
	},
	"exit": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"strconv.Atoi",    // 🟢 string-to-int conversion; pure function, no I/O.
	},
	"false": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
	},
	"du": {
		"context.Context",    // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",          // 🟢 error comparison; pure function, no I/O.
		"errors.New",         // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Sprintf",        // 🟢 string formatting; pure function, no I/O.
		"io.EOF",             // 🟢 sentinel error value; pure constant.
		"io/fs.FileInfo",     // 🟢 interface type for file information; no side effects.
		"math.Ceil",          // 🟢 pure arithmetic; rounds float up to nearest integer.
		"math.MaxInt64",      // 🟢 integer constant; used for overflow clamping.
		"strconv.FormatBool", // 🟢 bool→string conversion for pflag.Value; pure function.
		"strconv.ParseBool",  // 🟢 string→bool conversion for pflag.Value; pure function.
		"syscall.Stat_t",     // 🟢 Unix file stat struct for extracting Blocks/Nlink; read-only type, no I/O.
		"strconv.FormatInt",  // 🟢 formats byte counts; pure function, no I/O.
	},
	"find": {
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",                       // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",                       // 🟢 error comparison; pure function, no I/O.
		"errors.New",                      // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",                      // 🟢 error formatting; pure function, no I/O.
		"io.EOF",                          // 🟢 sentinel error value; pure constant.
		"io/fs.FileInfo",                  // 🟢 interface type for file information; no side effects.
		"io/fs.FileMode",                  // 🟢 file permission bits type; pure type.
		"io/fs.ModeCharDevice",            // 🟢 file mode bit constant for character devices; pure constant.
		"io/fs.ModeDevice",                // 🟢 file mode bit constant for block devices; pure constant.
		"io/fs.ModeDir",                   // 🟢 file mode bit constant for directories; pure constant.
		"io/fs.ModeNamedPipe",             // 🟢 file mode bit constant for named pipes; pure constant.
		"io/fs.ModeSetgid",                // 🟢 file mode bit constant for setgid; pure constant.
		"io/fs.ModeSetuid",                // 🟢 file mode bit constant for setuid; pure constant.
		"io/fs.ModeSocket",                // 🟢 file mode bit constant for sockets; pure constant.
		"io/fs.ModeSticky",                // 🟢 file mode bit constant for sticky bit; pure constant.
		"io/fs.ModeSymlink",               // 🟢 file mode bit constant for symlinks; pure constant.
		"io/fs.ReadDirFile",               // 🟢 read-only directory handle interface; no write capability.
		"math.Ceil",                       // 🟢 pure arithmetic; no side effects.
		"math.Floor",                      // 🟢 pure arithmetic; no side effects.
		"math.MaxInt64",                   // 🟢 integer constant; no side effects.
		"os.IsNotExist",                   // 🟢 checks if error is "not exist"; pure function, no I/O.
		"os.PathError",                    // 🟢 error type for path operations; pure type.
		"path/filepath.Dir",               // 🟢 returns the directory component of a path; pure function, no I/O.
		"path/filepath.IsAbs",             // 🟢 reports whether a path is absolute; pure function, no I/O.
		"path/filepath.ToSlash",           // 🟢 converts OS path separators to forward slashes; pure function, no I/O.
		"strconv.Atoi",                    // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.ErrRange",                // 🟢 sentinel error value for overflow; pure constant.
		"strconv.ParseInt",                // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.ParseUint",               // 🟢 string-to-unsigned-int conversion; pure function, no I/O.
		"strings.Contains",                // 🟢 substring search for {} detection in command validation; pure function, no I/O.
		"strings.HasPrefix",               // 🟢 pure function for prefix matching; no I/O.
		"strings.ReplaceAll",              // 🟢 replaces all {} occurrences in -exec/-execdir args; pure function, no I/O.
		"strings.Split",                   // 🟢 splits a string by separator into a slice; pure function, no I/O.
		"strings.ToLower",                 // 🟢 converts string to lowercase; pure function, no I/O.
		"time.Duration",                   // 🟢 duration type; pure integer alias, no I/O.
		"time.Hour",                       // 🟢 constant representing one hour; no side effects.
		"time.Minute",                     // 🟢 constant representing one minute; no side effects.
		"time.Second",                     // 🟢 constant representing one second; no side effects.
		"time.Time",                       // 🟢 time value type; pure data, no side effects.
		"unicode/utf8.DecodeRuneInString", // 🟢 decodes first UTF-8 rune from a string; pure function, no I/O.

	},
	"free": {
		"context.Context",    // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",          // 🟢 error comparison via chain (meminfo.ErrNotSupported); pure function, no I/O.
		"errors.New",         // 🟢 creates a sentinel error (noArgBool.Set rejects explicit values); pure function, no I/O.
		"fmt.Sprintf",        // 🟢 formats human-readable size strings; pure function, no I/O.
		"strconv.FormatUint", // 🟢 uint-to-string conversion; pure function, no I/O.
		"strconv.ParseFloat", // 🟢 re-parses an already-formatted size string to detect 1024-boundary promotion; pure function, no I/O.
		// Note: builtins/internal/meminfo symbols are exempt from this
		// allowlist (internal packages are not checked by the
		// builtinAllowedSymbols test).
	},
	"grep": {
		"bufio.NewScanner",  // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"bytes.IndexByte",   // 🟢 finds a byte in a byte slice; pure function, no I/O.
		"bytes.NewReader",   // 🟢 wraps a byte slice as an io.Reader; pure in-memory, no I/O.
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.New",        // 🟢 creates a simple error value; pure function, no I/O.
		"io.MultiReader",    // 🟢 combines multiple Readers into one sequential Reader; no I/O side effects.
		"io.NopCloser",      // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",     // 🟢 interface type; no side effects.
		"io.Reader",         // 🟢 interface type; no side effects.
		"os.O_RDONLY",       // 🟢 read-only file flag constant; cannot open files by itself.
		"regexp.Compile",    // 🟢 compiles a regular expression; pure function, no I/O. Uses RE2 engine (linear-time, no backtracking).
		"regexp.QuoteMeta",  // 🟢 escapes all special regex characters in a string; pure function, no I/O.
		"regexp.Regexp",     // 🟢 compiled regular expression type; no I/O side effects. All matching methods are linear-time (RE2).
		"strconv.Itoa",      // 🟢 int-to-string conversion; pure function, no I/O.
		"strconv.ParseBool", // 🟢 string-to-bool conversion; pure function, no I/O.
		"strings.Builder",   // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.Join",      // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
		"strings.Split",     // 🟢 splits a string by separator into a slice; pure function, no I/O.
	},
	"help": {
		"bytes.Buffer",    // 🟢 in-memory buffer to capture --help output from commands; no I/O side effects.
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"github.com/DataDog/datadog-agent/pkg/rshell/internal/version.Version", // 🟢 build version string; read-only package-level variable, no I/O.
		"strings.Builder", // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
	},
	"head": {
		"bufio.NewScanner", // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"context.Context",  // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",        // 🟢 error comparison; pure function, no I/O.
		"errors.New",       // 🟢 creates a simple error value; pure function, no I/O.
		"io.EOF",           // 🟢 sentinel error value; pure constant.
		"io.ReadSeeker",    // 🟢 interface type combining Reader and Seeker; no side effects.
		"io.Reader",        // 🟢 interface type; no side effects.
		"io.SeekCurrent",   // 🟢 whence constant for Seek(offset, SeekCurrent); pure constant.
		"os.O_RDONLY",      // 🟢 read-only file flag constant; cannot open files by itself.
		"strconv.ParseInt", // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
	},
	"journalctl": {
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"fmt.Sprintf",                     // 🟢 formats bounded journal storage counts in memory; no I/O.
		"io.WriteString",                  // 🟠 writes formatted journal lines to callCtx.Stdout; no filesystem access, delegates to Write.
		"io.Writer",                       // 🟢 interface type for the already-authorized output stream; no side effects.
		"strconv.ParseInt",                // 🟢 parses the bounded --lines value; pure function, no I/O.
		"strings.Builder",                 // 🟢 constructs one bounded output line in memory; no I/O.
		"time.Parse",                      // 🟢 parses an absolute RFC3339 timestamp; pure function, no I/O.
		"time.ParseDuration",              // 🟢 parses a bounded lookback duration; pure function, no I/O.
		"time.ParseInLocation",            // 🟢 parses a local timestamp using the runner's captured location; pure function, no I/O.
		"time.RFC3339Nano",                // 🟢 standard timestamp layout constant; no side effects.
		"time.Time",                       // 🟢 time value type; pure data, no side effects.
		"unicode.IsGraphic",               // 🟢 identifies non-graphic runes that must be escaped at the output boundary; pure function, no I/O.
		"unicode/utf8.DecodeRuneInString", // 🟢 decodes output text so control and malformed bytes can be escaped; pure function, no I/O.
		"unicode/utf8.RuneError",          // 🟢 replacement rune constant used to detect malformed UTF-8; no side effects.
		// Note: builtins/internal/sizeparse symbols are exempt from this
		// allowlist (internal packages are checked separately).
	},
	"jq": {
		"bufio.ErrTooLong",                // 🟢 sentinel distinguishing an over-long line from other read failures; pure error value.
		"bufio.NewScanner",                // 🟢 bounded line scanner for raw input; reads only from an already-authorized reader.
		"bytes.IndexByte",                 // 🟢 finds a byte in an in-memory slice; pure function, no I/O.
		"bytes.NewReader",                 // 🟢 wraps one bounded JSON value as an in-memory reader; no external I/O.
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"encoding/json.Decoder",           // 🟢 streaming JSON decoder type operating on an already-authorized reader.
		"encoding/json.Delim",             // 🟢 JSON delimiter token type; pure data, no side effects.
		"encoding/json.NewDecoder",        // 🟢 constructs a streaming JSON decoder around an existing reader; no I/O capability of its own.
		"encoding/json.Number",            // 🟢 preserves JSON number text for bounded exact parsing; pure string type.
		"encoding/json.RawMessage",        // 🟢 holds one bounded top-level JSON value as raw bytes for framing validation.
		"encoding/json.Unmarshal",         // 🟢 decodes a bounded filter string literal in memory; no filesystem or network I/O.
		"errors.As",                       // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",                       // 🟢 error comparison; pure function, no I/O.
		"errors.New",                      // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",                      // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                     // 🟢 string formatting; pure function, no I/O.
		"io.Closer",                       // 🟢 interface used to close sandbox-issued file handles; no capability by itself.
		"io.EOF",                          // 🟢 sentinel error value; pure constant.
		"io.ErrUnexpectedEOF",             // 🟢 sentinel for truncated JSON input; pure error value.
		"io.ErrShortWrite",                // 🟢 sentinel used when an output writer accepts only part of a bounded record.
		"io.Reader",                       // 🟢 interface type for already-authorized input; no side effects.
		"io.WriteString",                  // 🟠 writes bounded output to callCtx.Stdout; no filesystem access, delegates to Write.
		"io.Writer",                       // 🟢 interface type for the already-authorized output stream; no side effects.
		"math.Abs",                        // 🟢 pure floating-point absolute value operation; no side effects.
		"math.Copysign",                   // 🟢 pure sign transfer used to preserve negative zero; no side effects.
		"math.IsInf",                      // 🟢 pure predicate used to clamp overflowed generated numbers.
		"math.IsNaN",                      // 🟢 pure predicate used to reject non-finite generated numbers.
		"math.MaxFloat64",                 // 🟢 float bound constant used to saturate overflow like jq; pure constant.
		"math.MaxInt64",                   // 🟢 integer bound constant used to validate repetition counts.
		"math.MinInt64",                   // 🟢 integer bound constant used to validate repetition counts.
		"math.Mod",                        // 🟢 pure floating-point remainder operation; no side effects.
		"math.Trunc",                      // 🟢 pure floating-point truncation operation; no side effects.
		"math/big.Float",                  // 🟢 bounded in-memory numeric conversion type; no I/O or external side effects.
		"math/big.Int",                    // 🟢 bounded in-memory integer type used for exact arithmetic; no I/O or external side effects.
		"math/big.NewInt",                 // 🟢 constructs an in-memory integer from int64; pure arithmetic helper.
		"math/big.Rat",                    // 🟢 bounded in-memory rational type used for exact numeric comparison; no I/O.
		"os.PathError",                    // 🟢 inspects a sandbox error wrapper so its attacker-controlled path is not repeated in diagnostics.
		"sort.Strings",                    // 🟢 deterministically sorts bounded object keys in memory; no I/O.
		"strconv.ErrRange",                // 🟢 sentinel numeric range error; pure error value.
		"strconv.FormatFloat",             // 🟢 formats a finite float as JSON number text; pure conversion.
		"strconv.ParseFloat",              // 🟢 parses bounded decimal/exponent literals; pure conversion.
		"strings.Builder",                 // 🟢 bounded in-memory string construction; no I/O.
		"strings.Compare",                 // 🟢 lexicographically compares two strings; pure function.
		"strings.ContainsAny",             // 🟢 checks number syntax for decimal/exponent markers; pure function.
		"strings.Count",                   // 🟢 counts bounded separators before splitting; pure function.
		"strings.Cut",                     // 🟢 splits an encoded variable binding in memory; pure function.
		"strings.HasPrefix",               // 🟢 detects the internal marker for negative positional arguments; pure function.
		"strings.NewReader",               // 🟢 wraps bounded in-memory text as a reader; no external I/O.
		"strings.Repeat",                  // 🟢 creates a string only after the caller validates its bounded size; pure function.
		"strings.Split",                   // 🟢 splits bounded strings after pre-counting results; pure function.
		"unicode/utf8.DecodeRuneInString", // 🟢 decodes runes for bounded JSON serialization; pure function.
		"unicode/utf8.RuneError",          // 🟢 replacement rune used for each malformed raw-input byte.
		"unicode/utf8.RuneCountInString",  // 🟢 counts runes in bounded strings; pure function.
		"unicode/utf8.Valid",              // 🟢 validates bounded raw JSON bytes before malformed UTF-8 normalization.
		"unicode/utf8.ValidString",        // 🟢 fast-path validation before normalizing malformed raw strings.
	},
	"systemctl": {
		"context.Context",          // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"fmt.Errorf",               // 🟢 constructs bounded validation and backend errors in memory; no I/O.
		"slices.SortFunc",          // 🟢 deterministically sorts authorized selectors and bounded state values; pure in-memory operation.
		"strings.LastIndexByte",    // 🟢 locates a unit's final suffix separator; pure string inspection.
		"strings.Map",              // 🟢 sanitizes untrusted manager text at the output boundary; pure string transformation.
		"strings.SplitSeq",         // 🟢 streams bounded filter tokens without allocating an attacker-sized slice.
		"strings.ToValidUTF8",      // 🟢 replaces malformed manager output before display; pure string transformation.
		"unicode.IsGraphic",        // 🟢 identifies non-graphic manager output that must be sanitized; pure lookup.
		"unicode/utf8.ValidString", // 🟢 validates exact unit selectors before authorization or backend access; pure inspection.
		"errors.New",               // 🟢 constructs an error value; no I/O.
	},
	"logrotate": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",       // 🟢 error comparison; pure function, no I/O.
	},
	"lsof": {
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.New",                      // 🟢 creates a sentinel error (noArgBool.Set rejects explicit values); pure function, no I/O.
		"fmt.Errorf",                      // 🟢 error formatting; pure function, no I/O.
		"path/filepath.Clean",             // 🟢 normalizes a path lexically; pure function, no I/O.
		"path/filepath.Join",              // 🟢 lexically joins path components with the OS separator; pure function, no I/O.
		"path/filepath.Rel",               // 🟢 computes a relative path lexically; pure function, no I/O.
		"path/filepath.Separator",         // 🟢 OS path separator constant; pure constant, no I/O.
		"runtime.GOOS",                    // 🟢 current OS name constant; pure constant, no I/O.
		"slices.SortFunc",                 // 🟢 sorts a slice with a comparison function; pure function, no I/O.
		"strconv.Atoi",                    // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.Itoa",                    // 🟢 int-to-string conversion; pure function, no I/O.
		"strings.Builder",                 // 🟢 in-memory string builder used to sanitize output fields; pure, no I/O.
		"strings.Fields",                  // 🟢 splits a string on all whitespace; pure function, no I/O.
		"strings.HasPrefix",               // 🟢 pure function for prefix matching; no I/O.
		"strings.ReplaceAll",              // 🟢 replaces all occurrences of a substring; pure function, no I/O.
		"strings.Split",                   // 🟢 splits a string by separator into a slice; pure function, no I/O.
		"strings.TrimSpace",               // 🟢 removes leading/trailing whitespace; pure function, no I/O.
		"unicode.IsGraphic",               // 🟢 reports whether a rune is graphic (for output sanitization); pure function, no I/O.
		"unicode/utf8.DecodeRuneInString", // 🟢 decodes the first UTF-8 rune in a string (for output sanitization); pure function, no I/O.
		"unicode/utf8.RuneError",          // 🟢 sentinel rune constant for invalid UTF-8; pure constant, no I/O.
		// Note: builtins/internal/procfd symbols are exempt from this
		// allowlist (internal packages are not checked by the
		// builtinAllowedSymbols test).
	},
	"ls": {
		"context.Context",                    // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.New",                         // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Sprintf",                        // 🟢 string formatting; pure function, no I/O.
		"io/fs.DirEntry",                     // 🟢 interface type for directory entries; no side effects.
		"io/fs.FileInfo",                     // 🟢 interface type for file information; no side effects.
		"io/fs.ModeDir",                      // 🟢 file mode bit constant for directories; pure constant.
		"io/fs.ModeNamedPipe",                // 🟢 file mode bit constant for named pipes; pure constant.
		"io/fs.ModeSetgid",                   // 🟢 file mode bit constant for setgid; pure constant.
		"io/fs.ModeSetuid",                   // 🟢 file mode bit constant for setuid; pure constant.
		"io/fs.ModeSocket",                   // 🟢 file mode bit constant for sockets; pure constant.
		"io/fs.ModeSticky",                   // 🟢 file mode bit constant for sticky bit; pure constant.
		"io/fs.ModeSymlink",                  // 🟢 file mode bit constant for symlinks; pure constant.
		"os.O_RDONLY",                        // 🟢 read-only file flag constant; used on Windows for sandbox-aware file open.
		"runtime.GOOS",                       // 🟢 current OS name constant; pure constant, no I/O.
		"slices.Reverse",                     // 🟢 reverses a slice in-place; pure function, no I/O.
		"slices.SortFunc",                    // 🟢 sorts a slice with a comparison function; pure function, no I/O.
		"syscall.ByHandleFileInformation",    // 🟢 Windows file info struct for extracting nlink; read-only type, no I/O.
		"syscall.GetFileInformationByHandle", // 🟠 Windows API to query file metadata by handle; read-only, no I/O side effects.
		"syscall.Handle",                     // 🟢 Windows file handle type; pure type alias, no I/O.
		"syscall.Stat_t",                     // 🟢 Unix file stat struct for extracting UID/GID/nlink; read-only type, no I/O.
		"time.Time",                          // 🟢 time value type; pure data, no side effects.
		"strconv.FormatUint",                 // 🟢 formats numeric user/group IDs and link counts; pure function, no I/O.
		"strconv.FormatInt",                  // 🟢 formats file sizes; pure function, no I/O.
	},
	"ntfsdu": {
		"context.Context",             // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"encoding/json.MarshalIndent", // 🟢 serialises the scan result to indented JSON; pure function, no I/O.
		"fmt.Errorf",                  // 🟢 error formatting for --min parsing; pure function, no I/O.
		"path/filepath.IsAbs",         // 🟢 reports whether a relative-vs-absolute target operand needs WorkDir resolution; pure function, no I/O.
		"path/filepath.Join",          // 🟢 builds tree-node paths from parent path + basename; pure function, no I/O.
		"strconv.ParseInt",            // 🟢 parses the numeric part of a --min SIZE value; pure function, no I/O.
		"strings.TrimSpace",           // 🟢 trims a --min value before parsing; pure function, no I/O.
		"strings.TrimSuffix",          // 🟢 strips B/i/b suffixes from a --min value; pure function, no I/O.
		"time.RFC3339",                // 🟢 layout constant for formatting file created/modified times; pure constant.
		"time.Time",                   // 🟢 created/modified timestamp type carried from the engine into JSON; pure data type, no I/O.
		// Note: builtins/internal/ntfsmft symbols are exempt from this allowlist
		// (internal packages are not checked by the builtinAllowedSymbols test).
		"errors.New", // 🟢 constructs an error value; no I/O.
	},
	"pmap": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",       // 🟢 error comparison via chain (procmaps sentinels); pure function, no I/O.
		"errors.New",      // 🟢 creates the noArgBool "flag does not allow an argument" sentinel error; pure function, no I/O.
		"fmt.Errorf",      // 🟢 error formatting; pure function, no I/O.
		"strconv.Atoi",    // 🟢 string-to-int conversion; pure function, no I/O.
		// Note: builtins/internal/procmaps symbols are exempt from this
		// allowlist (internal packages are not checked by the
		// builtinAllowedSymbols test).
	},
	"ps": {
		"context.Context",       // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"fmt.Errorf",            // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",           // 🟢 formats numeric process metrics for display; pure function, no I/O.
		"math.IsInf",            // 🟢 rejects infinite process metric values before display/sort; pure arithmetic predicate.
		"math.IsNaN",            // 🟢 rejects NaN process metric values before display/sort; pure arithmetic predicate.
		"slices.SortStableFunc", // 🟢 stably sorts the in-memory process snapshot; pure slice operation, no I/O.
		"strconv.Atoi",          // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.FormatUint",    // 🟢 uint-to-string conversion for RSS/VSZ values; pure function, no I/O.
		"strconv.Itoa",          // 🟢 int-to-string conversion for PID/PPID/CPU values; pure function, no I/O.
		"strconv.ParseUint",     // 🟢 parses numeric UIDs for numeric sorting; pure function, no I/O.
		"strings.Fields",        // 🟢 splits a string on all whitespace; pure function, no I/O.
		"strings.ReplaceAll",    // 🟢 replaces all occurrences of a substring; pure function, no I/O.
		"strings.Split",         // 🟢 splits a string by separator into a slice; pure function, no I/O.
		"strings.ToLower",       // 🟢 normalizes fixed format/sort field names; pure function, no I/O.
		"strings.TrimPrefix",    // 🟢 removes a fixed error prefix before rendering; pure function, no I/O.
		"strings.TrimSpace",     // 🟢 removes leading/trailing whitespace; pure function.
		"time.Duration",         // 🟢 duration type used for elapsed-process formatting; pure integer alias, no I/O.
		"time.Second",           // 🟢 duration constant used to render elapsed process time; no side effects.
	},
	"printf": {
		"context.Context",      // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",            // 🟢 error type assertion; pure function, no I/O.
		"fmt.Sprintf",          // 🟢 string formatting; pure function, no I/O.
		"math.Inf",             // 🟢 returns positive or negative infinity; pure function, no I/O.
		"math.MaxUint64",       // 🟢 integer constant; no side effects.
		"math.NaN",             // 🟢 returns IEEE 754 NaN value; pure function, no I/O.
		"strconv.Atoi",         // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.ErrRange",     // 🟢 sentinel error value for overflow; pure constant.
		"strconv.IntSize",      // 🟢 platform int size constant (32 or 64); pure constant, no I/O.
		"strconv.Itoa",         // 🟢 int-to-string conversion; pure function, no I/O.
		"strconv.NumError",     // 🟢 error type for numeric conversion failures; pure type.
		"strconv.ParseFloat",   // 🟢 string-to-float conversion; pure function, no I/O.
		"strconv.ParseInt",     // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
		"strconv.ParseUint",    // 🟢 string-to-unsigned-int conversion; pure function, no I/O.
		"strings.Builder",      // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.ContainsRune", // 🟢 checks if a rune is in a string; pure function, no I/O.
		"strings.ReplaceAll",   // 🟢 replaces all occurrences of a substring; pure function, no I/O.
		"strings.ToLower",      // 🟢 converts string to lowercase; pure function, no I/O.
	},
	"pwd": {
		"context.Context",          // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",                // 🟢 error comparison; pure function, no I/O.
		"errors.New",               // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",               // 🟢 error formatting; pure function, no I/O.
		"io/fs.ModeSymlink",        // 🟢 file mode bit constant for symlinks; pure constant.
		"path/filepath.Clean",      // 🟢 normalizes a path lexically (collapses ".", "..", duplicate separators); pure function, no I/O.
		"path/filepath.Dir",        // 🟢 returns the directory component of a path; pure function, no I/O.
		"path/filepath.IsAbs",      // 🟢 reports whether a path is absolute; pure function, no I/O.
		"path/filepath.Join",       // 🟢 lexically joins path components with the OS separator; pure function, no I/O.
		"path/filepath.Separator",  // 🟢 OS path separator constant ('/' or '\\'); pure constant, no I/O.
		"path/filepath.VolumeName", // 🟢 returns the volume prefix of a path (e.g. "C:" on Windows, "" on Unix); pure function, no I/O.
		"strings.HasPrefix",        // 🟢 pure function for prefix matching; no I/O.
		"strings.IndexByte",        // 🟢 finds byte in string; pure function, no I/O.
		"strings.TrimPrefix",       // 🟢 removes a leading prefix from a string; pure function, no I/O.
	},
	"read": {
		"context.CancelFunc",                  // 🟢 cancellation function returned by context.WithTimeout; pure type.
		"context.Context",                     // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"context.DeadlineExceeded",            // 🟢 sentinel error value for context deadline expiry; pure constant.
		"context.WithTimeout",                 // 🟢 derives a timeout-bound context for read -t; no I/O itself.
		"errors.Is",                           // 🟢 error comparison; pure function, no I/O.
		"fmt.Errorf",                          // 🟢 error formatting; pure function, no I/O.
		"fmt.Fprint",                          // 🟠 writes the -p prompt to callCtx.Stderr; no filesystem access, delegates to Write.
		"fmt.Sprintf",                         // 🟢 string formatting; pure function, no I/O.
		"io.EOF",                              // 🟢 sentinel error value; pure constant.
		"io.Reader",                           // 🟢 interface type; no side effects.
		"math.IsInf",                          // 🟢 IEEE 754 infinity check; pure function, no I/O.
		"math.IsNaN",                          // 🟢 IEEE 754 NaN check; pure function, no I/O.
		"math.MaxInt64",                       // 🟢 maximum int64 constant; used to bound -t TIMEOUT before time.Duration conversion.
		"os.ErrDeadlineExceeded",              // 🟢 sentinel error returned by *os.File when SetReadDeadline fires; pure constant.
		"os.File",                             // 🟠 *os.File type used for type-asserting callCtx.Stdin to access SetReadDeadline/Stat; no constructors invoked.
		"strconv.Atoi",                        // 🟢 string-to-int conversion (used by orderedIntValue.Set for -n / -N); pure function, no I/O.
		"strconv.Itoa",                        // 🟢 int-to-string conversion (used by orderedIntValue.String for pflag default-value display); pure function, no I/O.
		"strconv.ParseFloat",                  // 🟢 string-to-float conversion (parses -t TIMEOUT seconds); pure function, no I/O.
		"strings.ContainsRune",                // 🟢 checks if a rune is in IFS; pure function, no I/O.
		"time.Duration",                       // 🟢 duration type; pure integer alias, no I/O.
		"time.Second",                         // 🟢 constant representing one second; no side effects.
		"time.Time",                           // 🟢 time value type; pure data, no side effects.
		"time.Unix",                           // 🟢 constructs an absolute Time at Unix-epoch + (sec, nsec); used by the cancellation watchdog to set an "in the past" SetReadDeadline. Pure constructor, no side effects.
		"unicode/utf8.DecodeLastRuneInString", // 🟢 decodes last UTF-8 rune in a string (trailing IFS-whitespace strip); pure function, no I/O.
		"unicode/utf8.DecodeRuneInString",     // 🟢 decodes first UTF-8 rune from a string; pure function, no I/O.
		"unicode/utf8.FullRune",               // 🟢 reports whether a byte slice begins with a complete UTF-8 rune; pure function, no I/O.
		"unicode/utf8.RuneSelf",               // 🟢 first byte value above which UTF-8 multi-byte encoding begins; pure constant.
		"unicode/utf8.UTFMax",                 // 🟢 maximum number of bytes in a UTF-8 encoding; pure constant.
		"golang.org/x/term.IsTerminal",        // 🟠 platform-specific isatty check (TIOCGETA / GetConsoleMode); used to gate -p prompt emission. Read-only inspection of the file descriptor; no I/O.
		"golang.org/x/sys/unix.Poll",          // 🟠 Unix poll(2) with timeout 0; non-consuming readability probe for read -t 0. Read-only descriptor state; no data transferred.
		"golang.org/x/sys/unix.PollFd",        // 🟢 PollFd struct passed to unix.Poll; pure data type, no I/O.
		"golang.org/x/sys/unix.POLLIN",        // 🟢 poll event constant for "data available to read"; pure constant.
		"golang.org/x/sys/unix.POLLHUP",       // 🟢 poll event constant for "peer hung up" (EOF-ready); pure constant.
	},
	"rm": {
		"context.Context",         // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",               // 🟢 error sentinel comparison (ErrRemoveBudgetExceeded); pure function, no I/O.
		"errors.New",              // 🟢 creates a simple error value; pure function, no I/O.
		"os.ModeSymlink",          // 🟢 file mode bit constant identifying a symlink; pure constant, no I/O.
		"path/filepath.Base",      // 🟢 returns the last element of a path; pure function, no I/O.
		"path/filepath.Separator", // 🟢 OS path separator constant ('/' or '\\'); pure constant, no I/O.
	},
	"sort": {
		"bufio.NewScanner",      // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"context.Context",       // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.New",            // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",            // 🟢 error formatting; pure function, no I/O.
		"io.NopCloser",          // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",         // 🟢 interface type; no side effects.
		"os.O_RDONLY",           // 🟢 read-only file flag constant; cannot open files by itself.
		"slices.SortFunc",       // 🟢 sorts a slice with a comparison function; pure function, no I/O.
		"slices.SortStableFunc", // 🟢 stable sort with a comparison function; pure function, no I/O.
		"strconv.Atoi",          // 🟢 string-to-int conversion; pure function, no I/O.
		"strings.Builder",       // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.IndexByte",     // 🟢 finds byte in string; pure function, no I/O.
	},
	"sed": {
		"bufio.NewScanner",    // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"bufio.Scanner",       // 🟢 scanner type for buffered input reading; no write or exec capability.
		"bytes.Buffer",        // 🟢 in-memory buffer that captures a rewritten file's output for -i before it is written back; no I/O side effects itself.
		"bytes.NewBuffer",     // 🟢 wraps a capacity-bounded byte slice for in-place input/output buffering; no I/O.
		"bytes.IndexByte",     // 🟢 finds a byte in a byte slice; pure function, no I/O.
		"bytes.NewReader",     // 🟢 wraps -i's fully-read original file bytes as an io.Reader for the scanner; pure in-memory, no I/O.
		"context.Background",  // 🟢 supplies a deliberately non-cancelled root for -i's restore-on-failure write, detached from the primary write's own (possibly just-cancelled) context; pure function, no I/O.
		"context.Context",     // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"context.WithTimeout", // 🟢 bounds -i's detached restore-on-failure write with its own cleanup deadline (restoreTimeout) so a stall on a slow/stalled backing store can't hang indefinitely; no I/O itself.
		"errors.As",           // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",           // 🟢 error comparison; used by readAllChunkedCancellable to detect a normal io.EOF; pure function, no I/O.
		"errors.New",          // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",          // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",         // 🟢 string formatting; pure function, no I/O.
		"io.Closer",           // 🟢 interface for releasing the identity-pinned read handle readAllBounded keeps open across -i's write-back; no capability by itself.
		"io.EOF",              // 🟢 sentinel error value signaling normal end of input; pure constant.
		"io.NopCloser",        // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",       // 🟢 interface type; no side effects.
		"io.Reader",           // 🟢 interface type; parameter type for the shared scan-loop helper (processReader) shared by the streaming and -i paths; no side effects by itself.
		"os.FileInfo",         // 🟢 file metadata interface returned by Stat; no I/O side effects.
		"os.O_RDONLY",         // 🟢 read-only file flag constant; cannot open files by itself.
		"regexp.Compile",      // 🟢 compiles a regular expression; pure function, no I/O. Uses RE2 engine (linear-time, no backtracking).
		"regexp.Regexp",       // 🟢 compiled regular expression type; no I/O side effects. All matching methods are linear-time (RE2).
		"strconv.Atoi",        // 🟢 string-to-int conversion; pure function, no I/O.
		"strconv.ParseInt",    // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
		"strings.Builder",     // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.IndexByte",   // 🟢 finds byte in string; pure function, no I/O.
		"strings.Join",        // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
		"time.After",          // 🟢 bounds -i's cancellation-independent identity-pin open (openPinBounded) so a stalled open(2) on the pin's context.Background() call can't hang forever; races against, does not perform, filesystem I/O itself.
		"time.Duration",       // 🟢 duration type; parameter type for openPinBoundedWithTimeout's explicit timeout (test-only override of the pinOpenTimeout constant); pure integer alias, no I/O.
		"time.Second",         // 🟢 constant representing one second; used to build restoreTimeout/pinOpenTimeout; no side effects.
		"strconv.FormatInt",   // 🟢 formats line numbers; pure function, no I/O.
	},
	"stat": {
		"context.Context",    // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",          // 🟢 unwraps PathError so the already-quoted operand is not duplicated; pure type inspection.
		"fmt.Sprintf",        // 🟢 formats the fallback filesystem type name; pure function, no I/O.
		"os.PathError",       // 🟢 path error wrapper type; inspected only to extract its inner error.
		"strconv.FormatUint", // 🟢 formats filesystem identifiers and counters; pure function, no I/O.
		"strconv.Quote",      // 🟢 safely quotes operand names and control characters; pure function, no I/O.
	},
	"strings_cmd": {
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",         // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",         // 🟢 error comparison; pure function, no I/O.
		"errors.New",        // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Sprintf",       // 🟢 string formatting; pure function, no I/O.
		"io.EOF",            // 🟢 sentinel error value; pure constant.
		"io.NopCloser",      // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",     // 🟢 interface type; no side effects.
		"os.O_RDONLY",       // 🟢 read-only file flag constant; cannot open files by itself.
		"os.PathError",      // 🟢 error type for filesystem path errors; pure type, no I/O.
		"strconv.FormatInt", // 🟢 int-to-string conversion; pure function, no I/O.
	},
	"tail": {
		"bufio.NewScanner",  // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",         // 🟢 error comparison; pure function, no I/O.
		"errors.New",        // 🟢 creates a simple error value; pure function, no I/O.
		"io.EOF",            // 🟢 sentinel error value; pure constant.
		"io.NopCloser",      // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",     // 🟢 interface type; no side effects.
		"io.Reader",         // 🟢 interface type; no side effects.
		"math.MinInt64",     // 🟢 integer constant; no side effects.
		"os.FileInfo",       // 🟢 file metadata interface returned by Stat; no I/O side effects.
		"os.O_RDONLY",       // 🟢 read-only file flag constant; cannot open files by itself.
		"strconv.ErrRange",  // 🟢 sentinel error value for overflow; pure constant.
		"strconv.ParseInt",  // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
		"strconv.ParseUint", // 🟢 string-to-unsigned-int conversion; pure function, no I/O.
	},
	"tee": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",       // 🟢 error comparison; pure function, no I/O.
		"errors.New",      // 🟢 creates a simple error value; pure function, no I/O.
		"io.Closer",       // 🟢 interface for releasing an already-issued handle; no capability by itself.
		"io.EOF",          // 🟢 sentinel error value; pure constant.
		"io/fs.ModeType",  // 🟢 file mode bit mask distinguishing regular files from special files; pure constant, used only to reject a non-regular write target before open.
		"io.Reader",       // 🟢 interface type; no side effects.
		"io.Writer",       // 🟢 interface type for writing; no side effects.
		"os.O_APPEND",     // 🟢 append-on-write flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
		"os.O_CREATE",     // 🟢 create-if-missing flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
		"os.O_TRUNC",      // 🟢 truncate-on-open flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
		"os.O_WRONLY",     // 🟢 write-only flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
		"os.PathError",    // 🟢 error type for filesystem path errors; pure type, no I/O.
	},
	"testcmd": {
		"context.Context",     // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"io/fs.FileInfo",      // 🟢 interface type for file information; no side effects.
		"io/fs.ModeNamedPipe", // 🟢 file mode bit constant for named pipes; pure constant.
		"io/fs.ModeSymlink",   // 🟢 file mode bit constant for symlinks; pure constant.
		"strconv.ParseInt",    // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
		"strings.TrimSpace",   // 🟢 removes leading/trailing whitespace; pure function.
	},
	"tr": {
		"context.Context",  // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",        // 🟢 error comparison; pure function, no I/O.
		"fmt.Sprintf",      // 🟢 string formatting; pure function, no I/O.
		"io.EOF",           // 🟢 sentinel error value; pure constant.
		"strconv.ParseInt", // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
	},
	"true": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
	},
	"truncate": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",       // 🟢 error comparison; pure function, no I/O.
		"os.ErrNotExist",  // 🟢 sentinel error value for missing file; pure constant.
	},
	"uname": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"runtime.GOOS",    // 🟢 current OS name constant; pure constant, no I/O.
		"strings.Join",    // 🟢 joins string slices; pure function, no I/O.
	},
	"uniq": {
		"bufio.NewScanner",  // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
		"bufio.SplitFunc",   // 🟢 type for custom scanner split functions; pure type, no I/O.
		"bytes.Equal",       // 🟢 compares two byte slices for equality; pure function, no I/O.
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"io.NopCloser",      // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",     // 🟢 interface type; no side effects.
		"io.Reader",         // 🟢 interface type; no side effects.
		"io.WriteString",    // 🟠 writes a string to a writer; no filesystem access, delegates to Write.
		"io.Writer",         // 🟢 interface type for writing; no side effects.
		"math.MaxInt64",     // 🟢 integer constant; no side effects.
		"os.O_RDONLY",       // 🟢 read-only file flag constant; cannot open files by itself.
		"strconv.ErrRange",  // 🟢 sentinel error value for overflow; pure constant.
		"strconv.FormatInt", // 🟢 int-to-string conversion; pure function, no I/O.
		"strconv.NumError",  // 🟢 error type for numeric conversion failures; pure type.
		"strconv.ParseInt",  // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
		"strings.HasPrefix", // 🟢 pure function for prefix matching; no I/O.
		"strings.Builder",   // 🟢 builds output strings in memory; no I/O.
	},
	"uptime": {
		"context.Context", // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"fmt.Sprintf",     // 🟢 string formatting for duration and load average; pure function, no I/O.
		"runtime.GOOS",    // 🟢 current OS name constant; used in "not supported" error message; pure constant, no I/O.
		"strings.Builder", // 🟢 efficient string concatenation for default output line; pure in-memory buffer, no I/O.
		"strings.Join",    // 🟢 concatenates pretty-format duration units with ", "; pure function, no I/O.
		"time.Time",       // 🟢 time value type; used for callCtx.Now display and boot-time formatting; pure data, no side effects.
		"time.Unix",       // 🟢 constructs absolute Time from Unix-epoch seconds for -s output; pure constructor, no I/O.
		// Note: builtins/internal/sysinfo symbols are exempt from this allowlist
		// (internal packages are not checked by the builtinAllowedSymbols test).
	},
	"vmstat": {
		"context.Context",   // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.New",        // 🟢 creates a bounded-sampling validation error; pure function, no I/O.
		"fmt.Errorf",        // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",       // 🟢 string formatting; pure function, no I/O.
		"math.IsNaN",        // 🟢 rejects NaN before float-to-uint conversion; pure function, no I/O.
		"math.MaxInt32",     // 🟢 integer constant; bounds the count operand before conversion to int; no side effects.
		"math.MaxUint64",    // 🟢 integer constant; saturates float-to-uint64 rate conversions instead of relying on out-of-range cast behavior; no side effects.
		"math.Round",        // 🟢 rounds a float64 to the nearest integer; used for CPU-tick percentage rounding; pure function, no I/O.
		"strconv.ParseUint", // 🟢 string-to-unsigned-int conversion; used to validate delay/count operands; pure function, no I/O.
		"strings.Join",      // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
		"strings.Repeat",    // 🟢 builds dash separators for the group header; pure function, no I/O.
		"time.Duration",     // 🟢 duration type; pure integer alias, no I/O.
		"time.NewTimer",     // 🟢 creates a timer that fires once after a duration; used for the sampling-interval wait; pure in-process scheduling, no I/O.
		"time.Second",       // 🟢 constant representing one second; no side effects.
		// Note: builtins/internal/vmstat symbols are exempt from this
		// allowlist (internal packages are not checked by the
		// builtinAllowedSymbols test).
	},
	"ss": {
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",                       // 🟢 error comparison; used to distinguish syscall.ENOENT from unexpected errors.
		"fmt.Errorf",                      // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",                     // 🟢 string formatting; pure function, no I/O.
		"strconv.FormatUint",              // 🟢 uint-to-string conversion; pure function, no I/O.
		"strconv.Itoa",                    // 🟢 int-to-string conversion; pure function, no I/O.
		"strings.Builder",                 // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"syscall.ENOENT",                  // 🟢 error constant for "no such file or directory"; used to distinguish IPv6-unavailable from genuine sysctl errors.
		"golang.org/x/sys/unix.SysctlRaw", // 🟠 macOS: reads kernel socket tables (read-only, no exec, no filesystem).
		// Note: builtins/internal/procnetsocket symbols are exempt from this allowlist
		// (internal packages are not checked by the builtinAllowedSymbols test).
	},
	"sha256sum": {
		"bufio.ErrTooLong",                // 🟢 sentinel for a scanner token exceeding its buffer; pure error value.
		"bufio.NewScanner",                // 🟢 bounded line-by-line checksum-manifest reading; no write or exec capability.
		"context.Context",                 // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"crypto/sha256.New",               // 🟢 constructs an in-memory SHA-256 digest; pure computation over caller-supplied bytes.
		"crypto/sha256.Size",              // 🟢 SHA-256 digest-size constant; no side effects.
		"encoding/hex.EncodeToString",     // 🟢 encodes a bounded digest as lowercase hexadecimal; pure function, no I/O.
		"errors.Is",                       // 🟢 error comparison; pure function, no I/O.
		"errors.New",                      // 🟢 creates bounded read/validation errors; pure function, no I/O.
		"fmt.Errorf",                      // 🟢 bounded error formatting; pure function, no I/O.
		"io.EOF",                          // 🟢 sentinel error value; pure constant.
		"io.Reader",                       // 🟢 interface type; no side effects.
		"os.ErrDeadlineExceeded",          // 🟢 sentinel returned when the stdin read deadline fires; used only to synchronize with context cancellation.
		"strings.Builder",                 // 🟢 bounded filename escaping and unescaping; pure in-memory buffer.
		"strings.HasPrefix",               // 🟢 detects checksum format prefixes; pure function, no I/O.
		"strings.NewReader",               // 🟢 wraps empty in-memory stdin; no external I/O.
		"strings.ToLower",                 // 🟢 normalizes hexadecimal digest text; pure function, no I/O.
		"time.Time",                       // 🟢 deadline value used to make blocking stdin reads cancellable; pure data type.
		"time.Unix",                       // 🟢 constructs a past deadline to wake a blocked stdin read on cancellation.
		"unicode/utf8.DecodeRuneInString", // 🟢 finds malformed filename bytes for unambiguous status escaping; pure function.
		"unicode/utf8.RuneError",          // 🟢 replacement-rune constant used to identify malformed UTF-8 bytes.
		"unicode/utf8.ValidString",        // 🟢 selects the fast path for valid UTF-8 status filenames; pure validation.
	},
	"wc": {
		"context.Context",         // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.As",               // 🟢 error type assertion; pure function, no I/O.
		"errors.Is",               // 🟢 error comparison; pure function, no I/O.
		"errors.New",              // 🟢 creates a simple error value; pure function, no I/O.
		"io.EOF",                  // 🟢 sentinel error value; pure constant.
		"io.NopCloser",            // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",           // 🟢 interface type; no side effects.
		"io.Reader",               // 🟢 interface type; no side effects.
		"os.O_RDONLY",             // 🟢 read-only file flag constant; cannot open files by itself.
		"strconv.FormatInt",       // 🟢 int-to-string conversion; pure function, no I/O.
		"syscall.EISDIR",          // 🟢 error number constant for "is a directory"; pure constant, no I/O.
		"syscall.Errno",           // 🟢 error type for system call error numbers; pure type, no I/O.
		"unicode.Cc",              // 🟢 control character category range table; pure data, no I/O.
		"unicode.Cf",              // 🟢 format character category range table; pure data, no I/O.
		"unicode.Co",              // 🟢 private-use character category range table; pure data, no I/O.
		"unicode.Is",              // 🟢 checks if rune belongs to a range table; pure function, no I/O.
		"unicode.IsGraphic",       // 🟢 reports whether rune is defined as a graphic character; pure function, no I/O.
		"unicode.Me",              // 🟢 enclosing mark category range table; pure data, no I/O.
		"unicode.Mn",              // 🟢 nonspacing mark category range table; pure data, no I/O.
		"unicode.Range16",         // 🟢 struct type for 16-bit Unicode ranges; pure data.
		"unicode.Range32",         // 🟢 struct type for 32-bit Unicode ranges; pure data.
		"unicode.RangeTable",      // 🟢 struct type for Unicode range tables; pure data.
		"unicode.Zs",              // 🟢 Unicode space separator category range table; pure data, no I/O.
		"unicode/utf8.DecodeRune", // 🟢 decodes first UTF-8 rune from a byte slice; pure function, no I/O.
		"unicode/utf8.RuneError",  // 🟢 replacement character returned for invalid UTF-8; constant, no I/O.
		"unicode/utf8.UTFMax",     // 🟢 maximum number of bytes in a UTF-8 encoding; constant, no I/O.
		"unicode/utf8.Valid",      // 🟢 checks if a byte slice is valid UTF-8; pure function, no I/O.
	},
	"xargs": {
		"bufio.NewReaderSize", // 🟢 buffered reader with caller-supplied size; pure wrapper, no I/O capability of its own.
		"bufio.Reader",        // 🟢 buffered reader type; pure data, no side effects.
		"bytes.NewReader",     // 🟢 in-memory empty stdin for sub-command (POSIX /dev/null analogue); pure constructor, no I/O.
		"context.Context",     // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"errors.Is",           // 🟢 error comparison; pure function, no I/O.
		"errors.New",          // 🟢 creates a simple error value; pure function, no I/O.
		"fmt.Errorf",          // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",         // 🟢 string formatting; pure function, no I/O.
		"io.EOF",              // 🟢 sentinel error value; pure constant.
		"io.NopCloser",        // 🟢 wraps a Reader with a no-op Close; no side effects.
		"io.ReadCloser",       // 🟢 interface type; no side effects.
		"io.Reader",           // 🟢 interface type; no side effects.
		"os.O_RDONLY",         // 🟢 read-only file flag constant; cannot open files by itself.
		"strconv.Atoi",        // 🟢 parses int flag values for the custom -n/-L pflag.Value type; pure parsing.
		"strconv.FormatBool",  // 🟢 stringifies the -0 flag's stored bool value; pure formatting, no I/O.
		"strconv.ParseBool",   // 🟢 parses bool flag values for the custom -0 pflag.Value type; pure parsing.
		"strconv.ParseUint",   // 🟢 parses hex/octal -d delimiter escapes (\xNN, \NNN); pure parsing, no I/O.
		"strings.Builder",     // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.ReplaceAll",  // 🟢 replaces all occurrences of REPLSTR in -I templates; pure function, no I/O.
		"strconv.Itoa",        // 🟢 formats integer argument counts; pure function, no I/O.
	},
	"ping": {
		"context.Context",         // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"context.WithTimeout",     // 🟢 creates a child context with a deadline; no filesystem or network I/O itself.
		"errors.Is",               // 🟢 error comparison via chain; pure function, no I/O.
		"fmt.Errorf",              // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",             // 🟢 string formatting; pure function, no I/O.
		"net.DefaultResolver",     // 🔴 default system DNS resolver; used for context-aware address lookup; network I/O is the explicit purpose of this builtin.
		"net.IPAddr",              // 🟢 resolved IP address struct (IP + Zone); pure data type, no I/O.
		"net.ParseIP",             // 🟢 parses an IP address string; pure function, no I/O.
		"math.IsInf",              // 🟢 IEEE 754 infinity check; pure function, no I/O.
		"math.IsNaN",              // 🟢 IEEE 754 NaN check; pure function, no I/O.
		"math.MaxInt64",           // 🟢 maximum int64 constant; used to compute time.Duration overflow boundary.
		"strconv.ParseFloat",      // 🟢 parses integer/float seconds for -W/-i flags; pure function, no I/O.
		"strings.Contains",        // 🟢 substring search; pure function, no I/O.
		"strings.IndexByte",       // 🟢 finds first occurrence of a byte in a string; pure function, no I/O.
		"strings.ToLower",         // 🟢 converts string to lowercase; pure function, no I/O.
		"syscall.EACCES",          // 🟢 POSIX errno constant for permission denied; pure constant, no I/O.
		"syscall.EPERM",           // 🟢 POSIX errno constant for operation not permitted; pure constant, no I/O.
		"syscall.EPROTONOSUPPORT", // 🟢 POSIX errno constant for protocol not supported; pure constant, no I/O.
		"time.Duration",           // 🟢 duration type alias (int64 nanoseconds); pure type, no I/O.
		"time.Millisecond",        // 🟢 constant representing one millisecond; no side effects.
		"time.ParseDuration",      // 🟢 parses Go duration strings (e.g. "1s"); pure function, no I/O.
		"time.Second",             // 🟢 constant representing one second; no side effects.
		"github.com/prometheus-community/pro-bing.NewPinger",  // 🔴 creates an ICMP pinger; network I/O is the explicit purpose of this builtin.
		"github.com/prometheus-community/pro-bing.NoopLogger", // 🟢 no-op logger that discards pro-bing internal messages; no side effects.
		"github.com/prometheus-community/pro-bing.Packet",     // 🟢 ICMP packet descriptor struct (received packet data); pure data type, no I/O.
		"github.com/prometheus-community/pro-bing.Pinger",     // 🔴 ICMP pinger struct; network I/O is the explicit purpose of this builtin.
		"github.com/prometheus-community/pro-bing.Statistics", // 🟢 ping round-trip statistics struct; pure data type, no I/O.
		"errors.New", // 🟢 constructs an error value; no I/O.
	},
	"ip": {
		"context.Context",      // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
		"fmt.Errorf",           // 🟢 error formatting; pure function, no I/O.
		"fmt.Sprintf",          // 🟢 string formatting; pure function, no I/O.
		"net.FlagBroadcast",    // 🟢 interface flag: supports broadcast; pure constant, no network connections.
		"net.FlagLoopback",     // 🟢 interface flag: is loopback; pure constant, no network connections.
		"net.FlagMulticast",    // 🟢 interface flag: supports multicast; pure constant, no network connections.
		"net.FlagPointToPoint", // 🟢 interface flag: point-to-point link; pure constant, no network connections.
		"net.FlagRunning",      // 🟢 interface flag: running state (Go 1.20+); pure constant, no network connections.
		"net.FlagUp",           // 🟢 interface flag: administratively up; pure constant, no network connections.
		"net.Flags",            // 🟢 interface flags type (uint); pure type, no network connections.
		"net.IP",               // 🟢 IP address type ([]byte); pure type, no network connections.
		"net.IPNet",            // 🟢 IP network struct (IP + Mask); pure type, no network connections.
		"net.Interface",        // 🟢 network interface descriptor (read-only OS struct); no network connections.
		"net.Interfaces",       // 🟠 read-only OS interface enumeration; no network connections or I/O.
		"strconv.FormatUint",   // 🟢 uint-to-string conversion for metric formatting; pure function, no I/O.
		"strconv.Itoa",         // 🟢 int-to-string conversion; pure function, no I/O.
		"strconv.ParseUint",    // 🟢 string-to-unsigned-int conversion for parseIPv4; pure function, no I/O.
		"strings.Builder",      // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
		"strings.Join",         // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
		"strings.Split",        // 🟢 splits a string by separator; pure function, no I/O.
		// Note: builtins/internal/procnetroute symbols are exempt from this allowlist
		// (internal packages are not checked by the builtinAllowedSymbols test).
		"errors.New", // 🟢 constructs an error value; no I/O.
	},
}

// callCtxAllFields lists every elevated-capability field of CallContext that
// the analyzer tracks. Plain data fields (Stdin, Stdout, Stderr, Now, InLoop,
// LastExitCode, Proc) are not tracked; they are universally available and
// carry no elevated capability. Systemd is tracked despite being a pointer
// because it provides privileged host introspection backends.
//
// Every entry must match a field name declared in builtins/builtins.go.
var callCtxAllFields = []string{
	"AccessFile",
	"AllowedPathsList",
	"AllowedSystemServicesList",
	"AuthorizeSystemd",
	"CanonicalizeRootPrefix",
	"ChangeDir",
	"CommandAllowed",
	"Env",
	"FileSystemStat",
	"FileIdentity",
	"GetVar",
	"HostPrefix",
	"IsDirEmpty",
	"LookupEnvVar",
	"LstatFile",
	"OpenDir",
	"OpenFile",
	"OpenRegularFile",
	"PortableErr",
	"ReadDir",
	"ReadDirLimited",
	"ReadlinkFile",
	"ReadableSystemServices",
	"Remove",
	"RunCommand",
	"RunCommandWithStdin",
	"SetVar",
	"StatFile",
	"Systemd",
	"Truncate",
	"TruncateToZeroIfAtLeast",
	"WorkDir",
	"WriteRegularFile",
}

// builtinPerCommandCallContextFields maps each builtin command name to the
// CallContext function fields it is permitted to access.
//
// Security invariants enforced by this map:
//   - Write-capable CallContext fields must appear only in remediation-only
//     entries.
//   - "RunCommand"/"RunCommandWithStdin" (command execution) must appear only
//     in the "find" and "xargs" entries.
//   - "ChangeDir" (working-directory mutation) must appear only in "cd".
//   - "SetVar" (shell-variable mutation) must appear only in "read".
var builtinPerCommandCallContextFields = map[string][]string{
	"awk": {
		"Env",
		"OpenFile",
		"PortableErr",
		"WorkDir",
	},
	"break":    {},
	"continue": {},
	"df":       {},
	"echo":     {},
	"ntfsdu":   {"WorkDir"},
	"exit":     {},
	"false":    {},
	"ping":     {},
	"printf":   {},
	"pmap":     {},
	"ps":       {},
	"ss":       {},
	"true":     {},
	"uname":    {},
	"uptime":   {},
	"vmstat":   {},

	"cat": {
		"OpenFile",
		"PortableErr",
	},
	"cd": {
		"ChangeDir",
		"HostPrefix",
		"LookupEnvVar",
		"LstatFile",
		"PortableErr",
		"ReadlinkFile",
		"StatFile",
		"WorkDir",
	},
	"cut": {
		"OpenFile",
		"PortableErr",
	},
	"du": {
		"FileIdentity",
		"LstatFile",
		"OpenDir",
		"PortableErr",
		"StatFile",
	},
	"find": {
		"CommandAllowed",
		"FileIdentity",
		"IsDirEmpty",
		"LstatFile",
		"OpenDir",
		"PortableErr",
		"RunCommand",
		"StatFile",
		"WorkDir",
	},
	"free": {},
	"grep": {
		"OpenFile",
		"PortableErr",
	},
	"head": {
		"OpenFile",
		"PortableErr",
	},
	"help": {
		"AllowedPathsList",
		"AllowedSystemServicesList",
		"CommandAllowed",
	},
	"ip": {
		"PortableErr",
	},
	"journalctl": {
		"AuthorizeSystemd",
		"Systemd",
	},
	"jq": {
		"OpenRegularFile",
		"PortableErr",
	},
	"systemctl": {
		"AuthorizeSystemd",
		"ReadableSystemServices",
		"Systemd",
	},
	"logrotate": {
		"PortableErr",
		"TruncateToZeroIfAtLeast",
	},
	"lsof": {
		"AllowedPathsList",
		"CanonicalizeRootPrefix",
		"HostPrefix",
	},
	"ls": {
		"LstatFile",
		"OpenFile",
		"PortableErr",
		"ReadDir",
		"ReadDirLimited",
		"ReadlinkFile",
		"StatFile",
	},
	"pwd": {
		"CanonicalizeRootPrefix",
		"HostPrefix",
		"LstatFile",
		"ReadlinkFile",
		"WorkDir",
	},
	"read": {
		"GetVar",
		"SetVar",
	},
	"rm": {
		"AllowedPathsList",
		"LstatFile",
		"PortableErr",
		"Remove",
		"StatFile",
	},
	"sed": {
		"AllowedPathsList",
		"OpenFile",
		"OpenRegularFile",
		"PortableErr",
		"WriteRegularFile",
	},
	"sha256sum": {
		"OpenRegularFile",
		"PortableErr",
	},
	"sort": {
		"OpenFile",
		"PortableErr",
	},
	"stat": {
		"FileSystemStat",
		"PortableErr",
	},
	"strings_cmd": {
		"OpenFile",
		"StatFile",
	},
	"tail": {
		"OpenFile",
		"PortableErr",
	},
	"tee": {
		"AllowedPathsList",
		"OpenFile",
		"PortableErr",
		"StatFile",
	},
	"testcmd": {
		"AccessFile",
		"LstatFile",
		"StatFile",
	},
	"tr": {
		"PortableErr",
	},
	"truncate": {
		"AllowedPathsList",
		"PortableErr",
		"Truncate",
	},
	"uniq": {
		"OpenFile",
		"PortableErr",
	},
	"wc": {
		"OpenFile",
		"PortableErr",
	},
	"xargs": {
		"CommandAllowed",
		"OpenFile",
		"RunCommand",
		"RunCommandWithStdin",
		"WorkDir",
	},
}

var builtinAllowedSymbols = []string{
	"bufio.ErrTooLong",            // 🟢 sentinel for a scanner token exceeding its buffer; pure error value.
	"bufio.NewReaderSize",         // 🟢 buffered reader with caller-supplied size; pure wrapper, no I/O capability of its own.
	"bufio.NewScanner",            // 🟢 line-by-line input reading (e.g. head, cat); no write or exec capability.
	"bufio.Reader",                // 🟢 buffered reader type; pure data, no side effects.
	"bufio.Scanner",               // 🟢 scanner type for buffered input reading; no write or exec capability.
	"bufio.SplitFunc",             // 🟢 type for custom scanner split functions; pure type, no I/O.
	"bytes.Buffer",                // 🟢 in-memory buffer to capture command output; no I/O side effects.
	"bytes.Equal",                 // 🟢 compares two byte slices for equality; pure function, no I/O.
	"bytes.Index",                 // 🟢 finds a byte sequence in a byte slice; pure function, no I/O.
	"bytes.IndexByte",             // 🟢 finds a byte in a byte slice; pure function, no I/O.
	"bytes.NewBuffer",             // 🟢 wraps a capacity-bounded byte slice; pure in-memory, no I/O.
	"bytes.NewReader",             // 🟢 wraps a byte slice as an io.Reader; pure in-memory, no I/O.
	"context.Background",          // 🟢 supplies a non-canceling context for parser and regex helper entrypoints; no side effects.
	"context.CancelFunc",          // 🟢 cancellation function returned by context.WithTimeout/WithCancel; pure type, no side effects beyond context tree.
	"context.Context",             // 🟢 deadline/cancellation plumbing; pure interface, no side effects.
	"context.DeadlineExceeded",    // 🟢 sentinel error value for context deadline expiry; pure constant.
	"context.WithTimeout",         // 🟢 creates a child context with a deadline; no filesystem or network I/O itself.
	"crypto/sha256.New",           // 🟢 constructs an in-memory SHA-256 digest; pure computation over caller-supplied bytes.
	"crypto/sha256.Size",          // 🟢 SHA-256 digest-size constant; no side effects.
	"encoding/hex.EncodeToString", // 🟢 encodes a bounded byte slice as lowercase hexadecimal; pure function, no I/O.
	"encoding/json.Decoder",       // 🟢 streaming JSON decoder type; acts only through its caller-supplied reader.
	"encoding/json.Delim",         // 🟢 JSON delimiter token type; pure data, no side effects.
	"encoding/json.MarshalIndent", // 🟢 ntfsdu: serialises the scan result to indented JSON; pure function, no I/O.
	"encoding/json.NewDecoder",    // 🟢 constructs a streaming decoder around an existing reader; no I/O capability of its own.
	"encoding/json.Number",        // 🟢 preserves JSON number source text; pure string type.
	"encoding/json.RawMessage",    // 🟢 raw JSON byte slice type; pure data with no I/O capability.
	"encoding/json.Unmarshal",     // 🟢 decodes bounded JSON data in memory; no filesystem or network I/O.
	"errors.As",                   // 🟢 error type assertion; pure function, no I/O.
	"errors.Is",                   // 🟢 error comparison; pure function, no I/O.
	"errors.New",                  // 🟢 creates a simple error value; pure function, no I/O.
	"fmt.Errorf",                  // 🟢 error formatting; pure function, no I/O.
	"fmt.Fprint",                  // 🟠 writes to a writer (e.g. callCtx.Stderr for read -p prompts); no filesystem access, delegates to Write.
	"fmt.Sprintf",                 // 🟢 string formatting; pure function, no I/O.
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version.Version", // 🟢 build version string; read-only package-level variable, no I/O.
	"github.com/prometheus-community/pro-bing.NewPinger",                   // 🔴 creates an ICMP pinger by resolving host; network I/O is the explicit purpose of the ping builtin.
	"github.com/prometheus-community/pro-bing.NoopLogger",                  // 🟢 no-op logger that discards pro-bing internal messages; no side effects.
	"github.com/prometheus-community/pro-bing.Packet",                      // 🟢 ICMP packet descriptor struct (received packet data); pure data type, no I/O.
	"github.com/prometheus-community/pro-bing.Pinger",                      // 🔴 ICMP pinger struct; network I/O is the explicit purpose of the ping builtin.
	"github.com/prometheus-community/pro-bing.Statistics",                  // 🟢 ping round-trip statistics struct; pure data type, no I/O.
	"golang.org/x/sys/unix.EINTR",                                          // 🟢 interrupted-system-call sentinel used to retry bounded poll waits.
	"golang.org/x/sys/unix.FD_SETSIZE",                                     // 🟢 maximum descriptor supported by Darwin select; pure constant.
	"golang.org/x/sys/unix.FdSet",                                          // 🟢 descriptor set used for a non-consuming Darwin readiness wait.
	"golang.org/x/sys/unix.F_GETFL",                                        // 🟢 descriptor-status query constant; pure constant.
	"golang.org/x/sys/unix.FcntlInt",                                       // 🟠 reads descriptor flags without mutating the descriptor.
	"golang.org/x/sys/unix.O_NONBLOCK",                                     // 🟢 nonblocking descriptor flag constant; pure constant.
	"golang.org/x/sys/unix.POLLIN",                                         // 🟢 poll event constant for "data available to read"; pure constant.
	"golang.org/x/sys/unix.POLLHUP",                                        // 🟢 poll event constant for "peer hung up" (EOF-ready); pure constant.
	"golang.org/x/sys/unix.Poll",                                           // 🟠 Unix poll(2) with timeout 0; non-consuming readability probe for read -t 0. Read-only descriptor state; no data transferred.
	"golang.org/x/sys/unix.PollFd",                                         // 🟢 PollFd struct passed to unix.Poll; pure data type, no I/O.
	"golang.org/x/sys/unix.Select",                                         // 🟠 Darwin select(2) readiness wait; non-consuming and bounded.
	"golang.org/x/sys/unix.Timeval",                                        // 🟢 timeout passed to Darwin select; pure data type.
	"golang.org/x/sys/unix.SysctlRaw",                                      // 🟠 macOS: reads kernel socket tables (read-only, no exec, no filesystem).
	"golang.org/x/term.IsTerminal",                                         // 🟠 platform-specific isatty check (TIOCGETA / GetConsoleMode); used to gate read -p prompt emission. Read-only inspection of the file descriptor; no I/O.
	"io.EOF",                                                               // 🟢 sentinel error value; pure constant.
	"io.ErrUnexpectedEOF",                                                  // 🟢 sentinel error for truncated input; pure constant.
	"io.ErrShortWrite",                                                     // 🟢 sentinel error for an incomplete writer operation; pure constant.
	"io.Closer",                                                            // 🟢 interface for releasing an already-issued handle; no capability by itself.
	"io.MultiReader",                                                       // 🟢 combines multiple Readers into one sequential Reader; no I/O side effects.
	"io.NopCloser",                                                         // 🟢 wraps a Reader with a no-op Close; no side effects.
	"io.ReadCloser",                                                        // 🟢 interface type; no side effects.
	"io.ReadSeeker",                                                        // 🟢 interface type combining Reader and Seeker; no side effects.
	"io.Reader",                                                            // 🟢 interface type; no side effects.
	"io.SeekCurrent",                                                       // 🟢 whence constant for Seek(offset, SeekCurrent); pure constant.
	"io.WriteString",                                                       // 🟠 writes a string to a writer; no filesystem access, delegates to Write.
	"io.Writer",                                                            // 🟢 interface type for writing; no side effects.
	"io/fs.DirEntry",                                                       // 🟢 interface type for directory entries; no side effects.
	"io/fs.ErrPermission",                                                  // 🟢 sentinel error for permission denied; pure constant.
	"io/fs.FileInfo",                                                       // 🟢 interface type for file information; no side effects.
	"io/fs.FileMode",                                                       // 🟢 file permission bits type; pure type.
	"io/fs.ModeCharDevice",                                                 // 🟢 file mode bit constant for character devices; pure constant.
	"io/fs.ModeDevice",                                                     // 🟢 file mode bit constant for block devices; pure constant.
	"io/fs.ModeDir",                                                        // 🟢 file mode bit constant for directories; pure constant.
	"io/fs.ModeNamedPipe",                                                  // 🟢 file mode bit constant for named pipes; pure constant.
	"io/fs.ModeSetgid",                                                     // 🟢 file mode bit constant for setgid; pure constant.
	"io/fs.ModeSetuid",                                                     // 🟢 file mode bit constant for setuid; pure constant.
	"io/fs.ModeSocket",                                                     // 🟢 file mode bit constant for sockets; pure constant.
	"io/fs.ModeSticky",                                                     // 🟢 file mode bit constant for sticky bit; pure constant.
	"io/fs.ModeSymlink",                                                    // 🟢 file mode bit constant for symlinks; pure constant.
	"io/fs.ModeType",                                                       // 🟢 file mode bit mask distinguishing regular files from special files; pure constant, used only to reject a non-regular write target before open.
	"io/fs.ReadDirFile",                                                    // 🟢 read-only directory handle interface; no write capability.
	"math.Abs",                                                             // 🟢 pure floating-point absolute value operation; no side effects.
	"math.Ceil",                                                            // 🟢 pure arithmetic; no side effects.
	"math.Copysign",                                                        // 🟢 transfers the sign of one float to another; pure function, no I/O.
	"math.Floor",                                                           // 🟢 pure arithmetic; no side effects.
	"math.Inf",                                                             // 🟢 returns positive or negative infinity; pure function, no I/O.
	"math.IsInf",                                                           // 🟢 IEEE 754 infinity check; pure function, no I/O.
	"math.IsNaN",                                                           // 🟢 IEEE 754 NaN check; pure function, no I/O.
	"math.MaxFloat64",                                                      // 🟢 float constant; no side effects.
	"math.MaxInt32",                                                        // 🟢 integer constant; no side effects.
	"math.MaxInt64",                                                        // 🟢 integer constant; no side effects.
	"math.MaxUint64",                                                       // 🟢 integer constant; no side effects.
	"math.MinInt64",                                                        // 🟢 integer constant; no side effects.
	"math.Mod",                                                             // 🟢 pure floating-point remainder operation; no side effects.
	"math.NaN",                                                             // 🟢 returns IEEE 754 NaN value; pure function, no I/O.
	"math.Pow",                                                             // 🟢 pure exponentiation; no side effects.
	"math.Round",                                                           // 🟢 rounds a float64 to the nearest integer; pure function, no I/O.
	"math.Trunc",                                                           // 🟢 truncates a float64 toward zero; pure function, no I/O.
	"math/big.Float",                                                       // 🟢 arbitrary-precision floating-point value used in bounded in-memory conversion; no I/O.
	"math/big.Int",                                                         // 🟢 arbitrary-precision integer value whose bit length is explicitly bounded; no I/O.
	"math/big.NewInt",                                                      // 🟢 constructs an in-memory integer from int64; pure helper.
	"math/big.Rat",                                                         // 🟢 arbitrary-precision rational value used for bounded comparison; no I/O.
	"net.DefaultResolver",                                                  // 🔴 default system DNS resolver; used for context-aware address lookup; network I/O is the explicit purpose of the ping builtin.
	"net.FlagBroadcast",                                                    // 🟢 interface flag constant: broadcast capability; pure constant, no network connections.
	"net.IPAddr",                                                           // 🟢 resolved IP address struct (IP + Zone); pure data type, no I/O.
	"net.FlagLoopback",                                                     // 🟢 interface flag constant: is loopback; pure constant, no network connections.
	"net.FlagMulticast",                                                    // 🟢 interface flag constant: multicast capability; pure constant, no network connections.
	"net.FlagPointToPoint",                                                 // 🟢 interface flag constant: point-to-point link; pure constant, no network connections.
	"net.FlagRunning",                                                      // 🟢 interface flag constant: running state (Go 1.20+); pure constant, no network connections.
	"net.FlagUp",                                                           // 🟢 interface flag constant: administratively up; pure constant, no network connections.
	"net.Flags",                                                            // 🟢 network interface flags type (uint); pure type, no network connections.
	"net.IP",                                                               // 🟢 IP address type ([]byte); pure type, no network connections.
	"net.IPNet",                                                            // 🟢 IP network struct (IP + Mask); pure type, no network connections.
	"net.ParseIP",                                                          // 🟢 parses an IP address string into a net.IP; pure function, no I/O.
	"net.Interface",                                                        // 🟢 OS network interface descriptor; read-only struct, no network connections.
	"net.Interfaces",                                                       // 🟠 read-only OS interface enumeration function; no network connections or writes.
	"os.ErrDeadlineExceeded",                                               // 🟢 sentinel error value for *os.File read/write deadline expiry; pure constant.
	"os.ErrNotExist",                                                       // 🟢 sentinel error value for "does not exist"; pure constant. Used by truncate -c to silently skip missing files.
	"os.File",                                                              // 🟠 *os.File type, used for type-asserting callCtx.Stdin to access SetReadDeadline/Stat (e.g. read -t timeout, TTY detection); no constructors invoked.
	"os.FileInfo",                                                          // 🟢 file metadata interface returned by Stat; no I/O side effects.
	"os.IsNotExist",                                                        // 🟢 checks if error is "not exist"; pure function, no I/O.
	"os.ModeSymlink",                                                       // 🟢 file mode bit constant identifying a symlink; pure constant, no I/O.
	"os.O_APPEND",                                                          // 🟢 append-on-write flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
	"os.O_CREATE",                                                          // 🟢 create-if-missing flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
	"os.O_RDONLY",                                                          // 🟢 read-only file flag constant; cannot open files by itself.
	"os.O_TRUNC",                                                           // 🟢 truncate-on-open flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
	"os.O_WRONLY",                                                          // 🟢 write-only flag constant; capability gate is callCtx.RemediationMode + the sandbox's write-open path, not the flag itself.
	"os.PathError",                                                         // 🟢 error type for filesystem path errors; pure type, no I/O.
	"path/filepath.Base",                                                   // 🟢 returns the last element of a path; pure function, no I/O.
	"path/filepath.Clean",                                                  // 🟢 normalizes a path lexically (collapses ".", "..", duplicate separators); pure function, no I/O.
	"path/filepath.Dir",                                                    // 🟢 returns the directory component of a path; pure function, no I/O.
	"path/filepath.FromSlash",                                              // 🟢 converts '/' to the OS separator without other normalisation; pure function, no I/O.
	"path/filepath.IsAbs",                                                  // 🟢 reports whether a path is absolute; pure function, no I/O.
	"path/filepath.Join",                                                   // 🟢 lexically joins path components with the OS separator; pure function, no I/O.
	"path/filepath.Rel",                                                    // 🟢 computes a relative path lexically; pure function, no I/O.
	"path/filepath.Separator",                                              // 🟢 OS path separator constant ('/' or '\\'); pure constant, no I/O.
	"path/filepath.ToSlash",                                                // 🟢 converts OS path separators to forward slashes; pure function, no I/O.
	"path/filepath.VolumeName",                                             // 🟢 returns the volume prefix of a path (e.g. "C:" on Windows, "" on Unix); pure function, no I/O.
	"regexp.Compile",                                                       // 🟢 compiles a regular expression; pure function, no I/O. Uses RE2 engine (linear-time, no backtracking).
	"regexp.QuoteMeta",                                                     // 🟢 escapes all special regex characters in a string; pure function, no I/O.
	"regexp.Regexp",                                                        // 🟢 compiled regular expression type; no I/O side effects. All matching methods are linear-time (RE2).
	"regexp/syntax.MatchNL",                                                // 🟢 parser flag enabling record-spanning character matches; pure constant.
	"regexp/syntax.OneLine",                                                // 🟢 parser flag giving anchors whole-record semantics; pure constant.
	"regexp/syntax.Parse",                                                  // 🟢 parses a regular expression into an in-memory AST; no I/O or side effects.
	"regexp/syntax.POSIX",                                                  // 🟢 parser mode restricting patterns to POSIX ERE syntax; pure constant.
	"runtime.GOOS",                                                         // 🟢 current OS name constant; pure constant, no I/O.
	"sort.Strings",                                                         // 🟢 sorts an in-memory string slice; pure transformation, no I/O.
	"slices.Reverse",                                                       // 🟢 reverses a slice in-place; pure function, no I/O.
	"slices.SortFunc",                                                      // 🟢 sorts a slice with a comparison function; pure function, no I/O.
	"slices.SortStableFunc",                                                // 🟢 stable sort with a comparison function; pure function, no I/O.
	"strconv.Atoi",                                                         // 🟢 string-to-int conversion; pure function, no I/O.
	"strconv.ErrRange",                                                     // 🟢 sentinel error value for overflow; pure constant.
	"strconv.FormatBool",                                                   // 🟢 bool-to-string conversion; pure function, no I/O.
	"strconv.FormatFloat",                                                  // 🟢 float-to-string conversion; pure function, no I/O.
	"strconv.FormatInt",                                                    // 🟢 int-to-string conversion; pure function, no I/O.
	"strconv.FormatUint",                                                   // 🟢 uint-to-string conversion; pure function, no I/O.
	"strconv.IntSize",                                                      // 🟢 platform int size constant (32 or 64); pure constant, no I/O.
	"strconv.Itoa",                                                         // 🟢 int-to-string conversion; pure function, no I/O.
	"strconv.NumError",                                                     // 🟢 error type for numeric conversion failures; pure type.
	"strconv.ParseBool",                                                    // 🟢 string-to-bool conversion; pure function, no I/O.
	"strconv.ParseFloat",                                                   // 🟢 string-to-float conversion; pure function, no I/O.
	"strconv.ParseInt",                                                     // 🟢 string-to-int conversion with base/bit-size; pure function, no I/O.
	"strconv.ParseUint",                                                    // 🟢 string-to-unsigned-int conversion; pure function, no I/O.
	"strconv.Quote",                                                        // 🟢 safely quotes strings with escaped control characters; pure function, no I/O.
	"strings.Builder",                                                      // 🟢 efficient string concatenation; pure in-memory buffer, no I/O.
	"strings.Compare",                                                      // 🟢 compares two strings lexicographically; pure function, no I/O.
	"strings.Contains",                                                     // 🟢 substring search; pure function, no I/O.
	"strings.ContainsAny",                                                  // 🟢 reports whether any selected byte/rune occurs; pure function, no I/O.
	"strings.ContainsRune",                                                 // 🟢 checks if a rune is in a string; pure function, no I/O.
	"strings.Count",                                                        // 🟢 counts non-overlapping substrings; pure function, no I/O.
	"strings.Cut",                                                          // 🟢 splits a string around the first separator; pure function, no I/O.
	"strings.Fields",                                                       // 🟢 splits a string on whitespace into a slice; pure function, no I/O.
	"strings.HasPrefix",                                                    // 🟢 pure function for prefix matching; no I/O.
	"strings.HasSuffix",                                                    // 🟢 pure function for suffix matching; no I/O.
	"strings.Index",                                                        // 🟢 substring search; pure function, no I/O.
	"strings.IndexByte",                                                    // 🟢 finds byte in string; pure function, no I/O.
	"strings.Join",                                                         // 🟢 concatenates a slice of strings with a separator; pure function, no I/O.
	"strings.LastIndexByte",                                                // 🟢 finds the final byte occurrence in a string; pure function, no I/O.
	"strings.Map",                                                          // 🟢 transforms runes in a string using a caller-supplied pure mapper; no I/O.
	"strings.NewReader",                                                    // 🟢 wraps a string as an io.Reader; pure in-memory, no I/O.
	"strings.ReplaceAll",                                                   // 🟢 replaces all occurrences of a substring; pure function, no I/O.
	"strings.Repeat",                                                       // 🟢 returns a string of n repetitions; pure function, no I/O.
	"strings.Split",                                                        // 🟢 splits a string by separator into a slice; pure function, no I/O.
	"strings.SplitN",                                                       // 🟢 splits a string with a caller-supplied result bound; pure function, no I/O.
	"strings.SplitSeq",                                                     // 🟢 iterates over string tokens without allocating a result slice; pure transformation.
	"strings.ToLower",                                                      // 🟢 converts string to lowercase; pure function, no I/O.
	"strings.ToUpper",                                                      // 🟢 converts string to uppercase; pure function, no I/O.
	"strings.ToValidUTF8",                                                  // 🟢 replaces invalid UTF-8 byte sequences; pure function, no I/O.
	"strings.Trim",                                                         // 🟢 removes a fixed set of leading/trailing characters; pure function, no I/O.
	"strings.TrimPrefix",                                                   // 🟢 removes a leading prefix from a string; pure function, no I/O.
	"strings.TrimSpace",                                                    // 🟢 removes leading/trailing whitespace; pure function.
	"strings.TrimSuffix",                                                   // 🟢 ntfsdu: strips B/i/b suffixes from a --min SIZE value; pure function, no I/O.
	"syscall.ByHandleFileInformation",                                      // 🟢 Windows file info struct for extracting nlink; read-only type, no I/O.
	"syscall.EACCES",                                                       // 🟢 POSIX errno constant for permission denied; pure constant, no I/O.
	"syscall.EISDIR",                                                       // 🟢 error number constant for "is a directory"; pure constant, no I/O.
	"syscall.EPERM",                                                        // 🟢 POSIX errno constant for operation not permitted; pure constant, no I/O.
	"syscall.EPROTONOSUPPORT",                                              // 🟢 POSIX errno constant for protocol not supported; pure constant, no I/O.
	"syscall.ENOENT",                                                       // 🟢 error constant for "no such file or directory"; pure constant, no I/O.
	"syscall.Errno",                                                        // 🟢 error type for system call error numbers; pure type, no I/O.
	"syscall.GetFileInformationByHandle",                                   // 🟠 Windows API to query file metadata by handle; read-only, no I/O side effects.
	"syscall.Handle",                                                       // 🟢 Windows file handle type; pure type alias, no I/O.
	"syscall.RawConn",                                                      // 🟠 pins a descriptor while a callback safely inspects it; no data access by itself.
	"syscall.Stat_t",                                                       // 🟢 file stat struct for extracting UID/GID/nlink; read-only type, no I/O.
	"sync.Once",                                                            // 🟢 one-time execution primitive used for idempotent reader cleanup.
	"time.After",                                                           // 🟢 fires a channel once after a duration, used to race/bound an otherwise-uninterruptible blocking call (e.g. sed -i's cancellation-independent identity-pin open); no I/O itself.
	"time.Duration",                                                        // 🟢 duration type; pure integer alias, no I/O.
	"time.Hour",                                                            // 🟢 constant representing one hour; no side effects.
	"time.Millisecond",                                                     // 🟢 constant representing one millisecond; no side effects.
	"time.Minute",                                                          // 🟢 constant representing one minute; no side effects.
	"time.NewTimer",                                                        // 🟢 creates a timer that fires once after a duration; pure in-process scheduling, no I/O.
	"time.Parse",                                                           // 🟢 parses timestamps according to a caller-supplied layout; pure function, no I/O.
	"time.ParseDuration",                                                   // 🟢 parses Go duration strings (e.g. "1s"); pure function, no I/O.
	"time.ParseInLocation",                                                 // 🟢 parses timestamps in a caller-supplied location; pure function, no I/O.
	"time.RFC3339",                                                         // 🟢 layout constant for formatting ntfs-du file created/modified times; pure constant.
	"time.RFC3339Nano",                                                     // 🟢 standard RFC3339 timestamp layout with optional fractional seconds; pure constant.
	"time.Second",                                                          // 🟢 constant representing one second; no side effects.
	"time.Time",                                                            // 🟢 time value type; pure data, no side effects.
	"time.Unix",                                                            // 🟢 constructs an absolute Time at Unix-epoch + (sec, nsec); pure constructor, no I/O or side effects.
	"unicode.Cc",                                                           // 🟢 control character category range table; pure data, no I/O.
	"unicode.Cf",                                                           // 🟢 format character category range table; pure data, no I/O.
	"unicode.Co",                                                           // 🟢 private-use character category range table; pure data, no I/O.
	"unicode.Is",                                                           // 🟢 checks if rune belongs to a range table; pure function, no I/O.
	"unicode.IsGraphic",                                                    // 🟢 reports whether rune is defined as a graphic character; pure function, no I/O.
	"unicode/utf8.ValidString",                                             // 🟢 reports whether a string contains valid UTF-8; pure function, no I/O.
	"unicode.Me",                                                           // 🟢 enclosing mark category range table; pure data, no I/O.
	"unicode.Mn",                                                           // 🟢 nonspacing mark category range table; pure data, no I/O.
	"unicode.Range16",                                                      // 🟢 struct type for 16-bit Unicode ranges; pure data.
	"unicode.Range32",                                                      // 🟢 struct type for 32-bit Unicode ranges; pure data.
	"unicode.RangeTable",                                                   // 🟢 struct type for Unicode range tables; pure data.
	"unicode.Zs",                                                           // 🟢 Unicode space separator category range table; pure data, no I/O.
	"unicode/utf8.DecodeLastRuneInString",                                  // 🟢 decodes the last UTF-8 rune from a string (used for trailing-IFS-whitespace stripping); pure function, no I/O.
	"unicode/utf8.DecodeRune",                                              // 🟢 decodes first UTF-8 rune from a byte slice; pure function, no I/O.
	"unicode/utf8.DecodeRuneInString",                                      // 🟢 decodes first UTF-8 rune from a string; pure function, no I/O.
	"unicode/utf8.FullRune",                                                // 🟢 reports whether a byte slice begins with a complete UTF-8 rune; pure function, no I/O.
	"unicode/utf8.RuneError",                                               // 🟢 replacement character returned for invalid UTF-8; constant, no I/O.
	"unicode/utf8.RuneCountInString",                                       // 🟢 counts decoded runes in a string; pure function, no I/O.
	"unicode/utf8.RuneSelf",                                                // 🟢 first byte value above which UTF-8 multi-byte encoding begins; constant, no I/O.
	"unicode/utf8.UTFMax",                                                  // 🟢 maximum number of bytes in a UTF-8 encoding; constant, no I/O.
	"unicode/utf8.Valid",                                                   // 🟢 checks if a byte slice is valid UTF-8; pure function, no I/O.

}
