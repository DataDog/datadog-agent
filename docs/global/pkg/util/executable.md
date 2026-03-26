# pkg/util/executable

## Purpose

`pkg/util/executable` provides helpers for locating the currently running agent executable and for resolving the absolute path of other programs on the system `PATH`. It fills a gap left by `os.Executable` and `filepath.EvalSymlinks`: the standard library functions don't always resolve symlinks consistently across platforms, which matters because the agent is often installed under a directory tree reached via symlinks.

---

## Key elements

**`Folder() (string, error)`** — returns the directory containing the current executable after resolving all symlinks. Uses `os.Executable()` followed by `filepath.EvalSymlinks()` then `filepath.Dir()`. Returns an error if either `os.Executable` or symlink resolution fails.

**`FolderAllowSymlinkFailure() (string, error)`** — same as `Folder`, but if symlink resolution fails it falls back to the unresolved path rather than returning an error. Useful in environments where the executable is accessed through a broken or dangling symlink (e.g. certain container setups).

**`ResolvePath(execName string) (string, error)`** — finds the named executable on `PATH` (via `exec.LookPath`) and returns its absolute path. Returns an error if the executable cannot be found or its absolute path cannot be determined.

---

## Usage

The package is used wherever the agent needs to locate companion binaries or config files relative to its own installation directory.

**`pkg/util/defaultpaths/path_nix.go`** and `path_darwin.go` — call `Folder()` to derive default paths for the config directory, log directory, and run directory relative to the agent binary location.

**`pkg/config/setup/config_nix.go`** — uses `Folder()` to locate the `dist/` directory bundled alongside the agent binary.

**`comp/trace/config/impl/config_windows.go`** and **`comp/core/gui/guiimpl/platform_windows.go`** — use `Folder()` to find companion executables in the same installation directory on Windows.

**`cmd/agent/subcommands/integrations/command.go`** — calls `ResolvePath("pip")` to locate the Python package manager used for integration management.

**`pkg/collector/python/init.go`** and **`pkg/collector/corechecks/embed/process/process_agent.go`** — use `FolderAllowSymlinkFailure()` or `Folder()` to build the path to embedded Python or to the process-agent binary.

### Example

```go
// Locate a config directory next to the binary
dir, err := executable.Folder()
if err != nil {
    return err
}
configPath := filepath.Join(dir, "dist", "datadog.yaml")

// Find an external tool
pipPath, err := executable.ResolvePath("pip3")
```

---

## Relationship to other packages

| Package | Relationship |
|---|---|
| `pkg/util/defaultpaths` ([docs](defaultpaths.md)) | `pkg/util/defaultpaths` is the primary consumer of this package. It calls `executable.Folder()` at package init time (via `GetInstallPath` and `GetDistPath`) to compute platform-specific default paths for config files, log directories, and the embedded `dist/` tree. Any code that needs default file paths should import `pkg/util/defaultpaths` rather than calling `executable.Folder()` directly. |
| `pkg/util/installinfo` ([docs](installinfo.md)) | `pkg/util/installinfo` reads the `install_info` YAML file located next to the agent config directory. It obtains the config directory path via the `model.Reader` passed to `installinfo.Get`, which ultimately derives from `defaultpaths.ConfPath` — a value computed using `executable.Folder()`. |
