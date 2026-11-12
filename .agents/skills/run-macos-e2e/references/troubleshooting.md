# Troubleshooting

Find the symptom, confirm the cause, then apply the fix. Most of these are
environment problems, not bugs in the branch under test, so rule them out
before changing code.

## Building the dmg

| Symptom | Cause | Fix |
|---|---|---|
| `rm_rf: wrong number of arguments (given 2, expected 1)` | omnibus ran under rbenv's Ruby 3.x. `RBENV_VERSION=system` doesn't reach `bundle exec omnibus` | Run the script unmodified, since it pins `/usr/bin` first in `PATH`. For manual `bundle` commands, use the same `PATH` (`setup.md`) |
| bundler tries to write into the system gem dir / permission denied | `omnibus/.bundle/config` is missing in this worktree | Create it with `BUNDLE_PATH: "~/.omnibus-gems"` (`setup.md`) |
| `ffi` / native extension load errors, `incompatible architecture` | `~/.omnibus-gems` extensions were built for the other CPU arch | Rerun `bundle install` for the target arch (`setup.md`) |
| x86_64 build: `undefined symbol: __isPlatformVersionAtLeast` | Command Line Tools compiler-rt missing or not fat | `xcode-select --install`. The script checks this up front |
| Every later Bazel command builds x86_64 binaries | An x86_64 build was killed with `SIGKILL` before its cleanup trap ran | Delete from `# --- x86_64 dmg build (temporary) ---` to the end of `user.bazelrc` |
| `racc` (or another native gem) fails in `bundle install`: `No rule to make target …/MacOSX.sdk/…/ruby-2.6.0/universal-darwin25/ruby/config.h` | The Command Line Tools' default `MacOSX.sdk` is newer than the OS and only ships the system Ruby headers for the next Darwin (for example `universal-darwin26`), while `/usr/bin/ruby` asks for `universal-darwin25`. The system Ruby's `rbconfig.rb` honours `SDKROOT` | Find the SDK that has the right directory (`ls /Library/Developer/CommandLineTools/SDKs/*/System/Library/Frameworks/Ruby.framework/Versions/2.6/usr/include/ruby-2.6.0/`) and run the `bundle install` from `setup.md` once with `SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX<ver>.sdk`. Setting `SDKROOT` for the build script itself does nothing, because the omnibus task drops it. The task's own `bundle install` then finds the gems installed and doesn't rebuild them |
| `Expected go1.x.y (from .go-version), but running go1.z…` right at the start | The scripts run `dda inv check-go-version`, like CI, and the first `go` on `PATH` is a different release | Install `~/sdk/go<.go-version>` (`setup.md`); the scripts put it first on `PATH` |
| `check_macos_version.sh`: `… (13.0) is higher than the minimimum acceptable (12.0)` | Built with a Go newer than `.go-version` (Go 1.27 stamps pure-Go binaries with minos 13.0), with a script copy that predates the version check | Same fix as the row above |
| `Dangling symlink` / `Outbound symlink` / `Absolute symlink` after the build | The install prefix holds a symlink that would break once installed. CI fails the dmg job on it too (`build_agent_dmg.sh`) | A real packaging bug: fix the software definition that creates the link |
| `hdiutil` / mount errors at the end of the build | A previous build left `/Volumes/Agent` mounted | `hdiutil detach /Volumes/Agent` |

## Starting the run

| Symptom | Cause | Fix |
|---|---|---|
| `E2E_MACOS_LOCAL_DMG must be an absolute path` / `does not name a readable .dmg` | Relative path. The test binary's working directory is not the repo root | Use `"$PWD/omnibus/pkg/<name>.dmg"` |
| `ssh: connect to host <vm-ip> port 22: Operation timed out` | VM is suspended, stopped, or rebooting | Parallels: `prlctl list -a`, then `prlctl resume <uuid>` or `prlctl start <uuid>`. Resume can turn into a reboot, so recheck Docker afterwards |
| `Too many authentication failures` / immediate disconnect | ssh offered every key loaded in your agent before the right one | Set `E2E_AWS_PRIVATE_KEY_PATH`. The test's `ssh` calls then add `-i <key> -o IdentitiesOnly=yes` |
| SetupSuite panics: `failed to init resource named: RemoteHost … ssh: this private key is passphrase protected` | `E2E_LOCAL_PRIVATE_KEY_PATH` is set, so the framework reads the key file itself and can't decrypt it | Unset it and pass the key as `E2E_AWS_PRIVATE_KEY_PATH`, with the key loaded in a private agent (next row) |
| framework SSH auth fails although `ssh -i <key>` works | The framework's `local` SSH client only uses an agent, and the launchd agent refused the key | Wrap the run in a private agent: `eval "$(ssh-agent -s)"; trap 'ssh-agent -k' EXIT; ssh-add <key>` |
| `starting fakeintake container on <vm-ip>: Cannot connect to the Docker daemon at unix://…/.colima/default/docker.sock` | Colima isn't running on the host, usually after a reboot | `ssh <host> 'eval "$(/opt/homebrew/bin/brew shellenv)"; colima start'` |
| `colima list` shows `Broken`; `colima start` fails with `ha.sock: connect: connection refused` | The VM rebooted uncleanly and left a stale `~/.colima/_lima/colima/ha.pid`. Its PID now belongs to an unrelated process, so Lima believes its host agent is still alive. Confirm with `ps -o comm= -p $(cat ~/.colima/_lima/colima/ha.pid)` | On the host: `rm -f ~/.colima/_lima/colima/ha.pid ~/.colima/_lima/colima/*.sock && colima start`. These are only runtime files; the disk image and config are untouched |
| A Bazel build of the test passed, but the run fails with `missing strict dependencies` | The E2E test target is tagged `manual`, so `bazel build //…:all` silently skips them | Build it by name: `bazel build //test/new-e2e/tests/agent-platform/tests:tests_test`, and add the missing deps to that package's `BUILD.bazel` |

## During the run

| Symptom | Cause | Fix |
|---|---|---|
| `TestNPMTracesConnection` skipped (LOCAL_VM) | Fakeintake runs on the target host, so the connection to it is routed over `lo0`, and the darwin network tracer never captures loopback (`isEligibleInterface` in `pkg/network/filter/packet_source_darwin.go`). No config option changes this | Expected. The test still runs on the AWS pool, where fakeintake is a separate host |
| `TestMacosInstallScriptUpgradeFromStable` skipped | Neither `E2E_MACOS_LOCAL_DMG` nor `E2E_PIPELINE_ID` is set, so there is no build to upgrade to | Set `E2E_MACOS_LOCAL_DMG` |
| `TestProcessAgentReportsProcessData` fails: `sudo launchctl submit -l ddprocsentinel …` exits with status 134 | `launchctl submit` aborts (SIGABRT) for every job on macOS 26 (seen on 26.6.1). CI runs Sonoma, where it works | Expected on a macOS 26 host; use a Sonoma VM or the AWS pool to cover this test. Check with `sudo launchctl submit -l e2eprobe -- /bin/sleep 5; echo $?` |
| Results look affected by a previous run (AWS pool, fast mode) | Your pool instance is never reverted on teardown, because local leases are persistent | Rerun with `E2E_REVERT_BEFORE_RUN=true` (`running.md`) |

Hosting fakeintake on your laptop so the VM reaches it over its real interface
looks like a fix for the NPM test, but managed macOS application firewalls
block incoming connections to unsigned binaries such as the test binary. It
would also make every subtest depend on the laptop firewall.

## AWS pool

The messages below come from `test/e2e-framework/resources/aws/ec2/pool/pool.go`.
Leases are S3 objects updated with conditional writes. Don't edit them by hand;
a wrong write can strand an instance or let two runs share one.

| Symptom | Meaning | Safe next step |
|---|---|---|
| `no idle instance available in pool of N` | Your instance is leased by a different stack, for example another run with a different `E2E_STACK_NAME_SUFFIX`, or one that's still running | Let the other run finish, or rerun with the same suffix as the run that holds it. A retry from the same stack takes the lease back automatically |
| `failed to revert local pool instance <id> before run: …` | `RevertInPlace` couldn't replace the root volume: the baseline AMI or snapshot no longer resolves, or the replace task failed | Read the wrapped error. Rerun without `E2E_REVERT_BEFORE_RUN` to keep working, and ask the pool owners about the baseline image |
| `instance <id> has no baseline image to revert to` | The lease record has no baseline AMI | A revert isn't possible for this instance; ask the pool owners |
| `lease for instance <id> changed hands … refusing to revert or release` | At teardown, another holder had taken the lease since this run acquired it | Nothing to do for this run. The other holder now owns the instance |
| `local pool provisioning requires a non-empty username` | The current OS user couldn't be resolved (`os/user.Current()` in `common/config/environment.go`), and an empty owner tag would provision a redundant Dedicated Host | Run `dda inv` as a normal local user account, not in a container or sandbox without a passwd entry |
| `WARNING: baseline image … is no longer resolvable, skipping revert` | Logged at release for a non-persistent lease; the instance is released without a revert | Informational for local runs |
