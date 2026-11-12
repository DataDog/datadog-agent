# One-time setup

## Building a dmg locally

Run the build scripts in `../scripts/` natively on an Apple Silicon Mac, from
inside the checkout or worktree you want to package. They mirror the unsigned
branch of CI's `.gitlab/build/package_build/build_agent_dmg.sh`: the same
`INSTALL_DIR`/`CONFIG_DIR`/`OMNIBUS_DIR`/`OMNIBUS_PACKAGE_DIR` layout, the same
`dda inv -- -e omnibus.build --skip-sign --skip-deps --go-mod-cache … --config-directory …
--install-directory … --base-dir …` call, and then CI's install-prefix symlink check
(`scripts/check_install_symlinks.sh`).

### What reaches the build

`omnibus.build` runs both `bundle install` and `bundle exec omnibus` with
`replace_env=True` (`tasks/omnibus.py` `bundle_install_omnibus`, `omnibus_run_task`). Only
the variables in `ENV_PASSHTROUGH` (`tasks/libs/common/omnibus.py`) get through, plus
what the task sets itself, for example `MACOSX_DEPLOYMENT_TARGET=12.0` and `SIGN_MAC`.
`PATH` and `HOME` pass; `RBENV_VERSION`, `BUNDLE_PATH`, `GOARCH`, `GOTOOLCHAIN`,
`SDKROOT` and `TMPDIR` don't. So everything the scripts pick (Ruby, Go, the x86_64
`go` shim) goes through `PATH`, and the gem location goes through
`omnibus/.bundle/config`.

| Script | Produces | Notes |
|---|---|---|
| `build_local_dmg_arm64.sh` | `omnibus/pkg/datadog-agent-<ver>.arm64.dmg` | native; ~2 min with warm Bazel/omnibus caches |
| `build_local_dmg_x86.sh` (+ `goshim/go`) | `omnibus/pkg/datadog-agent-<ver>.x86_64.dmg` | re-execs itself under Rosetta; Bazel cross-compiles; ~10 min |

The `.dmg` is stamped with the checkout's current commit. Uncommitted changes
are built in too, but the version string won't show them.

### Ruby: use the system Ruby 2.6, pinned through `PATH`

omnibus-ruby's `Builder#delete` passes a positional options hash to
`FileUtils.rm_rf`, which Ruby 3.x rejects with
`rm_rf: wrong number of arguments (given 2, expected 1)`. The dmg CI job doesn't
install a Ruby (`.gitlab/build/package_build/dmg.yml`); it uses whatever the
`macos:sonoma-*` runner image provides. Locally the scripts use the macOS system
Ruby 2.6.

`RBENV_VERSION=system` alone doesn't help, because it doesn't reach the
`bundle exec omnibus` that `dda inv` spawns. The scripts pin Ruby by putting
`/usr/bin` ahead of the rbenv shims:
`PATH="$HOME/.gem/ruby/2.6.0/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"`. Use the
same `PATH` for any manual `bundle` command.

### Go: the release `.go-version` names

CI installs exactly `.go-version` with `gimme`, then runs `dda inv check-go-version`
(`.macos_setup_go` in `dmg.yml`). Locally the build uses the first `go` on `PATH`,
and `GOTOOLCHAIN=auto` keeps a newer local one. A Go newer than `.go-version` can
fail the build in `check_macos_version.sh` (`... (13.0) is higher than the minimimum
acceptable (12.0)`), because Go 1.27 stamps pure-Go binaries with minos 13.0. The
scripts put `~/sdk/go<.go-version>/bin` first on `PATH` when it exists, then run
`dda inv check-go-version` like CI and stop on a mismatch. Install that release
once with `go install golang.org/dl/go<ver>@latest && go<ver> download`.

### Bundler: `omnibus/.bundle/config`

This file is gitignored, so create it in every worktree. The scripts stop if it's
missing:

```yaml
---
BUNDLE_PATH: "~/.omnibus-gems"
```

Without it, bundler tries to write gems into the system gem directory and fails.
Exporting `BUNDLE_PATH` doesn't help, because it never reaches bundler. CI does the
same thing with a job-local path: `bundle config set --local path 'vendor/bundle'`
(`.cache_omnibus_ruby_deps` in `.gitlab/.pre/common/shared.yml`). Locally, the
shared `~/.omnibus-gems` saves reinstalling the gems in every worktree.

`--skip-deps` only skips `dda inv go.deps` (the Go module download); CI passes it
too. The task still runs `bundle install` in `omnibus/` on every build, after
deleting `omnibus/Gemfile.lock`. Gems already in `~/.omnibus-gems` are reused, so
this is quick, but any native gem that is missing or newly pinned is compiled during
the build. If that compile fails (see the `SDKROOT` row in `troubleshooting.md`), run
the `bundle install` below by hand first.

### Gems: match their native extensions to the build arch

`~/.omnibus-gems` is shared across worktrees and keyed by Ruby version, not by
CPU architecture. Its native extensions (`ffi_c.bundle` and others) belong to
whichever architecture last ran `bundle install`. Check before building:

```bash
file $(find ~/.omnibus-gems/ruby/2.6.0 -name '*.bundle' | head -3)   # arm64 or x86_64
```

If they don't match the dmg you're about to build, reinstall them:

```bash
(cd omnibus && PATH="$HOME/.gem/ruby/2.6.0/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH" \
  BUNDLE_PATH="$HOME/.omnibus-gems" bundle install)
```

For an x86_64 reinstall, run the same command under `arch -x86_64`. Then check
that the gems landed under `ruby/2.6.0/`; if they landed under `ruby/3.x`,
Ruby 3 was picked up instead. If a native gem fails to compile with
`No rule to make target …/universal-darwin<N>/ruby/config.h`, prefix the command
with `SDKROOT=` pointing at an SDK that ships those headers (`troubleshooting.md`).
This is a local workaround: CI's runner image has a matching SDK.

### x86_64-only requirements

- The Command Line Tools must provide a fat `libclang_rt.osx.a`
  (`xcode-select --install`). The hermetic LLVM toolchain's copy is arm64-only,
  and the CPython link needs `__isPlatformVersionAtLeast`. The script checks for
  this and exits with a clear message.
- The script temporarily appends `--platforms=//bazel/platforms:macos_x86_64`
  and two `--linkopt` lines to the checkout's gitignored `user.bazelrc`, behind a
  marker, and strips them on exit. If a build is killed with `SIGKILL`, delete
  everything from `# --- x86_64 dmg build (temporary) ---` to the end of
  `user.bazelrc` by hand. Otherwise every later Bazel command in that checkout
  builds for x86_64.

### How the local build differs from CI's signed build

The scripts always pass `--skip-sign`. CI signs release and RC builds (`SIGN: true`
in `dmg.yml`); other pipelines build unsigned as well. A signed build differs in
ways that matter when you read test results:

| Step | Signed CI build | Local build |
|---|---|---|
| Binaries | `--skip-sign` unset, so the task sets `SIGN_MAC=true`; `agent.rb` sets `code_signing_identity` and `entitlements_file`, and `datadog-agent-finalize.rb` runs `codesign -o runtime --entitlements … --timestamp` on every executable (hardened runtime) | unsigned, no hardened runtime, no entitlements |
| `.pkg` | `package :pkg` sets `signing_identity` (Developer ID Installer); `PackagerPKGNotarizer` (`omnibus/lib/project_extension.rb`) notarizes and staples it | unsigned, not notarized |
| `.dmg` | notarized and stapled by `build_agent_dmg.sh` after the build | unsigned, not notarized |
| Keychain | ephemeral `build.keychain`, created and destroyed by `build_agent_dmg.sh` | none |

So a local build can't show signing problems: hardened runtime denying something
an unsigned binary is allowed to do, entitlement gaps, or Gatekeeper rejecting the
`.pkg` once it's extracted from the `.dmg`. The E2E install path, where
`install_mac_os.sh` with `DD_DMG_PATH` runs `hdiutil attach` and then
`installer -pkg` as root over SSH, installs the unsigned build fine. A copy that
macOS has quarantined (downloaded through a browser, for example) can still be
blocked by Gatekeeper. A failure that only shows up in CI, or only on a signed
build, can come from this gap rather than the code.

CI's artifacts are only `omnibus/pkg/*.dmg` and `version-manifest.json`. A local
build also leaves the `.pkg` and `*.metadata.json` files in `omnibus/pkg/`.

## LOCAL_VM target host

Any macOS host you can reach over SSH works: a Parallels or UTM VM, or a
spare Mac. It needs:

- **SSH key auth** for a user with **passwordless sudo**. The suites run
  `sudo` for installs, `launchctl` and file checks.
- **Homebrew** at `/opt/homebrew`, plus **Colima** (or another Docker
  daemon). Every remote command runs
  `eval "$(/opt/homebrew/bin/brew shellenv)"` first, because a
  non-interactive SSH session doesn't load the login profile.
- **Internet access** to pull `public.ecr.aws/datadog/fakeintake`.
- Nothing else of value on it. `SetupSuite` removes any existing Agent install,
  and the install suite's `TestZZUninstallAgent` uninstalls the Agent at the
  end. The upgrade suite leaves the upgraded Agent installed.
- For the upgrade suite, access to `install.datadoghq.com` and the public `.dmg`,
  to install the released Agent it upgrades from.

The suites start a fakeintake container on the host (`docker run -p 0:80`)
and point the Agent at `<vm-ip>:<published-port>` (`macosStartLocalFakeIntake`).

Take a VM snapshot once the host is set up. LOCAL_VM never reverts the host,
so restoring that snapshot is your clean run.

## AWS mac pool

- An AWS SSO profile with E2E access. Use the `-8h` role
  (`sso-agent-sandbox-account-admin-8h`), which re-authenticates every 8 hours
  instead of every hour. Run `dda inv e2e.setup --team=<github-team>` once on
  the host (not in `dda env dev`) to configure the profile and the E2E key pair.
- A local (non-CI) run only ever uses **your own** pool instance, found by the
  `dd:macos-e2e-pool-owner=<your OS username>` tag (from `os/user.Current()`, so the same
  local account always gets the same instance). If you don't own one yet,
  the first run provisions one on a new Dedicated Host, which bills a 24-hour
  minimum. Get the user's go-ahead before that first run.
- `mac1.metal` is x86_64, so build the x86_64 dmg for it.
