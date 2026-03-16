# `_run_on_windows_dev_env` — Full Pipeline Analysis

## Call chain for `dda inv -- test --only-modified-packages --host windows`

```
local machine
└── dda inv test (tasks/gotest.py)
    ├── get_modified_packages()          # runs `git diff` locally → ["pkg/util/json"]
    ├── build package_list               # ["./pkg/util/json/."]
    ├── build win_cmd                    # "inv test --build-stdlib --targets=./pkg/util/json/."
    └── _run_on_windows_dev_env(ctx, name="windows-dev-env", command=win_cmd)
        │
        ├── ctx.cd('./test/e2e-framework')
        ├── ctx.run("dda inv -- aws.show-vm --stack-name=windows-dev-env", hide=True)
        │   └── returns JSON → RemoteHost(address, user, port)
        │
        ├── build docker_command:
        │   "docker exec -it windows-dev-env powershell 'inv test --build-stdlib --targets=./pkg/util/json/.'"
        │
        ├── build ssh_command:
        │   "ssh Administrator@<ip> -p <port> -t \"docker exec -it windows-dev-env powershell '...' \""
        │
        └── ctx.run(ssh_command, pty=True, warn=True)
            │
            [local PTY] ──ssh -t──► [Windows VM PTY via conhost.exe]
                                         └── docker exec -it
                                               └── [container PTY]
                                                     └── powershell 'inv test ...'
```

---

## Bug 1 — Terminal is cleared on every call

### Root cause: triple PTY allocation

The pipeline creates **three** stacked TTYs:

| Layer | Where | Flag |
|---|---|---|
| invoke PTY | local process | `pty=True` in `ctx.run` |
| SSH PTY | Windows VM | `-t` in ssh command |
| Docker PTY | container | `-t` in `docker exec -it` |

When the SSH session opens, Windows `conhost.exe` initializes and sends ANSI
escape sequences to set up its display:

```
ESC[2J   ← clear screen
ESC[m    ← reset attributes
ESC[H    ← cursor to home
ESC]0;Administrator: C:\Windows\system32\conhost.exe ← set window title
ESC[?25h ← show cursor
```

Because `pty=True` passes raw escape codes through to the local terminal, these
sequences actually execute on the developer's terminal — clearing the screen.

### Fix

Remove the `-t` flag from the `ssh` command and from `docker exec`.
`pty=True` on the invoke side already ensures streaming output locally; the
remote PTY chain is unnecessary and only triggers Windows console initialization.

```python
# Before
ssh_command = "ssh Administrator@<ip> -p <port> -t \"docker exec -it ...\""

# After
ssh_command = "ssh Administrator@<ip> -p <port> \"docker exec -i ...\""
# Note: -i kept on docker exec so stdin is still connected
```

---

## Bug 2 — `gotestsum` not found in container

### Root cause: missing `Invoke-BuildScript` wrapper

The `test` task builds `win_cmd` as a bare `inv test` command and passes it
directly to `_run_on_windows_dev_env`. Inside the container, `gotestsum` (and
other build tools) are installed under paths only added to `$PATH` by
`Invoke-BuildScript` in `tasks/winbuildscripts/common.ps1`.

The `windows-dev-env.run` task correctly wraps every command:
```powershell
. ./tasks/winbuildscripts/common.ps1
Invoke-BuildScript -InstallDeps $false -Command { <user command> }
```

This sets up `$PATH`, Go environment variables, vcpkg, etc. Without it,
`gotestsum`, `go`, and other tools are missing from `$PATH`.

The `test` task skips this wrapper and calls `_run_on_windows_dev_env` with
the raw `inv test ...` string, so the container sees an empty `$PATH` and
fails immediately.

### Fix

Wrap `win_cmd` in the same `Invoke-BuildScript` envelope used by the `run` task:

```python
win_cmd = f"inv test --build-stdlib --targets={','.join(package_list)}"
wrapped = f'. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps \\$false -Command {{{win_cmd}}}'
windows_run(ctx, name="windows-dev-env", command=wrapped)
```

---

## Summary of fixes

| Bug | File | Change |
|---|---|---|
| Terminal clear | `tasks/windows_dev_env.py` | Remove `-t` from ssh command; use `docker exec -i` (not `-it`) |
| `gotestsum` not found | `tasks/gotest.py` | Wrap `win_cmd` with `Invoke-BuildScript` before passing to `_run_on_windows_dev_env` |
