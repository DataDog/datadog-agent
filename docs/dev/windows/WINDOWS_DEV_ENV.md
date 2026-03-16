# Windows Development Environment

Invoke tasks to create and manage a remote Windows development environment on AWS.

## `windows-dev-env.start`

Creates a remote Windows development environment and keeps it in sync with local changes.

**Usage:** `dda inv windows-dev-env.start [--name <name>]`

**Steps:**

1. Runs `dda inv -- setup` to initialize the local tooling.
2. Checks that `rsync` is installed locally (installs via `brew install rsync` on macOS if missing).
3. Creates a Windows VM on AWS using the `aws.create-vm` task with:
   - AMI: `ami-09b68440cb06b26d6` (Windows Server 2022)
   - Instance type: `t3.xlarge`
   - Architecture: `x86_64`
   - No agent pre-installed
   - Stack name: `windows-dev-env` (or the value of `--name`)
4. Parses the SSH connection string from the command output to extract the host address.
5. Disables Windows Defender on the VM and triggers a reboot.
6. Polls the VM via SSH until it has fully rebooted (checks that `Get-MpComputerStatus` returns "Invalid class", confirming Defender is gone).
7. Checks whether a Docker container named `windows-dev-env` is already running on the VM.
8. If not running, starts the container:
   ```
   docker run -m 16384 -v C:\mnt:c:\mnt:rw -w C:\mnt\datadog-agent -t -d \
     --name windows-dev-env datadog/agent-buildimages-windows_x64:ltsc2022 tail -f /dev/null
   ```
9. Runs `git pull` inside the container to fetch the latest `datadog-agent` and speed up the initial sync.
10. Rsyncs the local repository to the VM at `C:\mnt\datadog-agent\` (respecting `.gitignore`, excluding `.git/`).
11. Runs the full dependency installation inside the container:
    - `Invoke-BuildScript -InstallTestingDeps $true -InstallDeps $true`
    - `pre-go-build.ps1`
    - `dda inv -- -e tidy`
12. Prints the total elapsed time.
13. Starts a local file watcher (`watchdog`) that re-runs rsync automatically on every local file change, until interrupted with `Ctrl+C`.

---

## `windows-dev-env.stop`

Destroys the remote Windows development environment.

**Usage:** `dda inv windows-dev-env.stop [--name <name>]`

**Steps:**

1. Runs `aws.destroy-vm --stack-name=<name>` from the `test/e2e-framework` directory, which tears down the Pulumi stack and all associated AWS resources.

---

## `windows-dev-env.run`

Syncs local changes and runs a single command on the remote Windows development environment.

**Usage:** `dda inv windows-dev-env.run [--name <name>] --command <powershell-command>`

**Steps:**

1. Retrieves the VM connection info (address, user, port) by running `aws.show-vm --stack-name=<name>`.
2. Rsyncs the local repository to the VM (same rsync command as `start`).
3. Runs the provided command inside the `windows-dev-env` Docker container via:
   ```
   ssh Administrator@<host> -p <port> -t \
     "docker exec -it windows-dev-env powershell \
       '. ./tasks/winbuildscripts/common.ps1; Invoke-BuildScript -InstallDeps $false -Command {<command>}'"
   ```
4. Exits with the same exit code returned by the remote command.
