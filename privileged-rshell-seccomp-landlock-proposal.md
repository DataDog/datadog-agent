# Privileged rshell seccomp and Landlock proposal

## Summary

Use the allowlists already present in every signed rshell task. No backend or
protobuf changes are required.

```text
PAR
 └─ sends original signed envelope
      └─ privileged helper
           ├─ verifies backend signature
           ├─ intersects backend and local allowlists
           ├─ converts effective paths into Landlock rules
           └─ starts a one-shot worker
                ├─ applies Landlock
                ├─ installs the static seccomp denylist
                ├─ runs the rshell command
                └─ exits
```

## 1. Keep the existing authorization contract

Continue using:

- `system_inputs.remote_action.allowed_paths`
- `system_inputs.remote_action.allowed_commands`
- `inputs.elevatableCommands`
- The optional root-provisioned helper policy's local allowlists

The helper independently verifies the original signed envelope. When the
optional local policy exists, it intersects its allowlists with the signed
backend values to produce effective `AllowedPaths`, `AllowedCommands`, and
`ElevatableCommands`; otherwise, the signed backend values are effective.

PAR continues forwarding the signed envelope through
`pkg/privateactionrunner/bundles/remoteaction/rshell/run_command.go`. It should
not construct or send a separate sandbox policy.

## 2. Start a fresh worker for every command

Refactor the rshell privileged helper so the long-lived server does not execute
commands directly.

For each verified request:

1. Verify the signed envelope.
2. Compute the effective backend and local allowlists.
3. Parse and validate the rshell program.
4. Start the same binary in an internal `privileged-worker` mode.
5. Send the verified command and effective allowlists through an inherited
   pipe.
6. Wait for the worker's bounded response.
7. Terminate the worker on cancellation or timeout.

Before accepting requests, the helper also resolves the configured service
account, installs only that account's supplementary groups, and permanently
drops its real, effective, and saved GID. It retains real UID 0 only for the
existing narrowly scoped effective-UID elevation callback.

The worker processes exactly one command and exits. This prevents Landlock or
seccomp restrictions from contaminating subsequent requests.

Only the policy/result pipes and standard descriptors should be inherited. Use
close-on-exec for everything else and a parent-death signal so workers cannot
outlive the helper unexpectedly.

## 3. Convert effective paths into Landlock rules

Translate the existing path suffixes:

| Path specification | Landlock grants |
| --- | --- |
| `/path:ro` or `/path` | `READ_FILE`, `READ_DIR` |
| `/path:rw` | Read rights plus the specific mutation rights rshell supports |

The initial `:rw` mapping should likely include:

- `WRITE_FILE`
- `TRUNCATE`
- `MAKE_REG`

Do not grant these unless an rshell operation requires them:

- `EXECUTE`
- `MAKE_DIR`
- `REMOVE_DIR`
- `MAKE_SYM`
- `MAKE_SOCK`
- `MAKE_FIFO`
- `MAKE_CHAR`
- `MAKE_BLOCK`
- `REFER`
- `IOCTL_DEV`

Confirm the minimal rights needed by redirection, `truncate`, and `logrotate`
before fixing the mapping.

The ruleset's handled mask must include every operation the policy promises to
prevent, including rights not granted to any path. Otherwise, Landlock leaves
those operations unrestricted.

For example:

```text
handled everywhere:
    execute, read, write, truncate, remove-file/remove-dir,
    create-file/create-dir/create-special, symlink, refer

/path:ro grants:
    read-file, read-dir

/path:rw grants:
    read-file, read-dir, write-file, truncate,
    create-regular-file
```

`REMOVE_FILE` remains handled but is not granted by ordinary `:rw` paths.
Current `:rw` semantics cover redirection, `truncate`, and the truncation-only
`logrotate` builtin; they do not authorize deletion. A future journal-vacuum
grant must be a separate trusted action-specific rule.

An empty effective path list creates a ruleset with no backend-derived grants.
The worker still adds an exact read/write rule for `/dev/null`, preserving
rshell's documented redirection behavior without exposing the rest of `/dev`.

## 4. Preserve secure path resolution

Landlock rules should be attached to safely opened path descriptors, not paths
resolved with an unchecked `os.Open`.

The policy builder should:

- Accept only absolute Linux paths.
- Normalize paths consistently with the existing helper intersection.
- Resolve effective paths beneath a locally authorized root.
- Prevent `..`, symlink, and magic-link escapes.
- Use the same opened object for validation and `landlock_add_rule` to avoid
  path-replacement races.
- Close policy descriptors before executing the command.

A missing or inaccessible path must never broaden access. It can either fail
worker setup or be omitted with an explicit sandbox warning, causing attempts
to access it to fail.

The existing rshell `allowedpaths` enforcement remains enabled. Landlock is an
additional kernel-enforced layer, not a replacement.

The rule implementation must validate and attach each rule using the same
`O_PATH` descriptor. Validating a pathname, closing it, and letting a library
reopen it would reintroduce a path-replacement race.

Some builtins intentionally use fixed kernel paths outside `AllowedPaths`.
Preserve those contracts with typed, command-dependent read grants derived
from the effective `allowed_commands` list:

| Effective command | Additional trusted grant |
| --- | --- |
| `rshell:ps` | read-only directory `/proc` |
| `rshell:ss`, `rshell:ip` | read-only directory `/proc/net` |
| `rshell:df` | read-only file `/proc/self/mountinfo` |
| `rshell:uname` | its five read-only `/proc/sys/kernel/*` files |

Do not widen exact trusted files to broad parent directories. `journalctl`
needs typed grants for its machine-ID file, journal directories, and control
socket, plus a distinct removal grant for vacuuming. Add those only when the
privileged-helper protocol also transports the already-authorized systemd
service actions; the current verified command does not.

## 5. Apply Landlock inside the worker

Worker initialization order:

1. Decode the bounded parent-provided request.
2. Parse the command.
3. Construct the effective Landlock ruleset.
4. Set `no_new_privs`.
5. Apply Landlock to all Go runtime threads.
6. Close policy descriptors.
7. Install seccomp.
8. Create the rshell runner.
9. Run the command with selective elevation.
10. Return the result and exit.

Because remediation requires truncation, the initial implementation should
require at least Landlock ABI 3. Privileged execution should fail closed when
Landlock is unavailable, disabled, or too old. Nonprivileged rshell execution
remains unaffected.

Landlock restrictions continue to apply while the worker temporarily has
effective UID 0.

## 6. Install the static seccomp denylist

The supplied syscall list becomes a version-controlled policy in the rshell
helper. It is not derived from or controllable by the backend.

Use a denylist filter with:

- Default action: allow
- Denied syscall action: initially `EPERM`
- `no_new_privs`: enabled
- `SECCOMP_FILTER_FLAG_TSYNC`: enabled
- Unknown syscall names: setup or build error
- Filter installation failure: privileged execution error

Install seccomp after Landlock so the Landlock setup syscalls do not need
exemptions.

The initial reviewed deny policy is:

- Process/image control: deny `fork`, `vfork`, `clone3`, `execve`, and
  `execveat`; allow `clone` only with the exact Go runtime thread flags for the
  current architecture.
- Credentials/capabilities: deny `setuid`, `setgid`, `setreuid`, `setregid`,
  `setresgid`, `setgroups`, `setfsuid`, `setfsgid`, `capset`, and `prctl`.
  `setresuid` is the deliberate exception used by selective elevation.
- Namespaces/mounts: deny `unshare`, `setns`, `mount`, `umount2`,
  `pivot_root`, `chroot`, `open_tree`, `move_mount`, `fsopen`, `fsconfig`,
  `fsmount`, and `mount_setattr`.
- Kernel/process inspection: deny `bpf`, `perf_event_open`, `ptrace`,
  `process_vm_readv`, `process_vm_writev`, `process_madvise`,
  `process_mrelease`, `pidfd_getfd`, `pidfd_send_signal`, and `kcmp`.
- Keyrings/modules/reboot: deny `keyctl`, `add_key`, `request_key`,
  `init_module`, `finit_module`, `delete_module`, legacy module calls,
  `kexec_load`, `kexec_file_load`, and `reboot`.
- Devices and global configuration: deny `mknod`, `mknodat`, `ioctl`,
  ownership/mode/xattr/timestamp mutation, process signaling/scheduler changes,
  system-clock changes, swap, accounting, quota, hostname/domain-name,
  raw-I/O, and fanotify calls.
- Additional kernel attack surface: deny all three `io_uring` syscalls,
  `userfaultfd`, `open_by_handle_at`, and `name_to_handle_at`.

`prctl` and `ioctl` are denied only after parent-death signaling, Landlock, and
seccomp setup are complete. The exact `clone` exception is version-sensitive
and must be revalidated whenever the Go toolchain is upgraded.

Allowing `setresuid` means an exploit in the one-shot worker could request the
same effective-UID transition as the trusted callback. It cannot remove the
already-installed Landlock or seccomp layers, so those layers—not the temporary
unprivileged EUID alone—are the containment boundary. Tests must exercise the
deny policy again after restoring effective UID 0.

Before committing the list, check every entry against:

- Go runtime requirements
- `setresuid`, which selective elevation requires
- rshell filesystem, process, journald, and networking builtins
- Linux `amd64` and `arm64` syscall tables

Particularly dangerous calls can use `KILL_PROCESS` instead of `EPERM`, but
that should be an explicit decision per group.

## 7. Command-dependent narrowing

The initial implementation already derives the fixed `/proc` exceptions in
section 4 from the effective command list. Ordinary path mutation rights remain
derived from `:ro` and `:rw`, while `allowed_commands` remains the rshell
command gate.

A later tightening can additionally use the parsed program:

- Read-only program: omit all mutation grants, even for `:rw` paths.
- `truncate`: grant write, truncate, and possibly create-file.
- `logrotate`: grant only the mutation rights it actually uses.
- Output redirection: grant write, truncate, or create as appropriate.

This is optional defense-in-depth and does not require a backend change.

## 8. Testing

In the rshell repository:

- Test `:ro` and `:rw` Landlock translations.
- Test that backend/local path intersection remains authoritative.
- Test empty and missing path behavior.
- Test symlink and path-replacement attempts.
- Test allowed reads and mutations.
- Test equivalent operations outside allowed paths fail.
- Run two sequential commands with different policies to prove isolation.
- Verify effective UID 0 cannot bypass Landlock.
- Verify denied syscalls remain denied during elevated callbacks.
- Verify cancellation kills the worker.
- Verify unsupported Landlock kernels fail closed.
- Exercise the policy on both `amd64` and `arm64`.

Extend the existing root integration test to cover both Landlock and seccomp.

In `datadog-agent`:

- Bump the `github.com/DataDog/rshell` dependency.
- Update Go sums and Bazel dependency metadata.
- Preserve the existing signed-envelope wire compatibility test.
- Build using `dda inv rshell.build` and
  `dda inv privateactionrunner.build`.
- Run the relevant packages with `dda inv test`.

## Implementation status

The reviewed policy is implemented on
`rfc/privileged-rshell-helper/landlock-seccomp`. The remaining cross-repository
integration step is to publish a reachable rshell revision, update the Agent's
module version to that revision, and run the Agent packaging builds.

## References

- [Linux Landlock userspace API](https://www.kernel.org/doc/html/latest/userspace-api/landlock.html)
- [Linux seccomp filter documentation](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)
- [Go-Landlock](https://github.com/landlock-lsm/go-landlock)
