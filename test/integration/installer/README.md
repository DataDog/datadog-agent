# OCI package-hook CLI E2E checks

`package_hooks.py` runs **built installer binaries** through install, upgrade,
remove, experiment, config and extension CLI paths with actual local OCI layouts.
It is a local, disposable-container E2E tier, not a substitute for the cloud
installer suites' real systemd/MSI/Agent-health coverage.

Build an unchanged baseline and a modified Linux installer with the repository's
`dda inv installer.build --no-cgo --output-bin=...` task. Select the same `GOARCH`
as the test image. Then run:

```sh
dda inv installer.test-package-hooks \
  --installer=/absolute/path/new-installer \
  --baseline-installer=/absolute/path/baseline-installer \
  --image=<locally-available-image-id-or-digest> \
  --output=/absolute/path/receipts
```

The image must provide `/usr/local/bin/python3`, a POSIX shell, a default root
user and a normal writable Linux root filesystem. A pinned Python Alpine image
suffices. The host needs Docker, `zstd` and `openssl`. No images are pulled by the
test. `--docker-host` selects an existing daemon without changing the user's
Docker context; `--scenario` accepts a comma-separated subset for iteration.

Each scenario gets a new non-privileged, network-disabled container. Only the
two binaries, fixtures and runner are bind-mounted, all read-only. Installer
writes to `/etc`, `/opt` and `/usr` stay inside that container. The runner refuses
to invoke the installer without its container guard. Cleanup targets only the
randomly named container created for the scenario, never other containers,
images, volumes or clusters.

The baseline × package matrix uses the **real compiled APM injector recipe's**
`dd-host-install` creation/removal to distinguish fallback from a no-op. The
fixture disables injection itself; it is not an apm-inject package rollout or an
injector functional test. Hook-bearing Agent fixtures embed the actual baseline
installer to catch accidental delegation to a binary that does not support
package hooks. A loopback TLS recording intake captures real installer telemetry
without sending anything outside the container. All lifecycle operations,
filesystem/database transitions and child-process cancellation are real.

Receipts include CLI arguments, exit codes, output, event/context records,
telemetry, container image ID and installer hashes. Failures stop the run and
retain receipts. No mocked lifecycle or unit-only test is counted as a pass.
