# Build the Agent packages

## Building on your system (Linux and Mac)

The project will be built locally and provide a .tar.xz tarball (in the omnibus/pkg folder)
with the resulting artifacts by default on linux.
This artifact is the expected source when building a container image.
You can chose to generate an installable package in the form of a deb/rpm artifact by
providing a `OMNIBUS_FORCE_PACKAGES` environment variable during the build.
On macOS, a dmg artifact will always be generated.
Most of the files will be copied or created under the same installation path of
the final package, `/opt/datadog-agent`, but if you run Omnibus from Linux, some
files will be copied into `/etc`. This means two things:

 * If you already have a Datadog Agent installed, you might need to move it to a
   different location before operating Omnibus.
 * You need root privileges to build the packages (Linux only).

For these reasons, if you're running Linux we strongly suggest to use a dedicated
virtual machine or a Docker container where Omnibus can safely move things around
the filesystem without disrupting anything.

To run Omnibus and build the package, make the `/opt` folder world readable and run:

```
dda inv -- omnibus.build --base-dir=$HOME/.omnibus
```

On Mac, you might want to skip the signing step by running:

```
dda inv -- omnibus.build --base-dir=$HOME/.omnibus --skip-sign
```

The path you pass with the `--base-dir` option will contain the sources
downloaded by Omnibus in the `src` folder, the binaries cached after building
those sources in the `cache` folder and the final deb/rpm/dmg artifacts in the
`pkg` folder. You can fine tune an Omnibus run passing more options, see
`dda inv -- omnibus.build --help` for the list of all the available options.

**Note:** it's strongly advised to pass `--base-dir` and point to a directory
outside the Agent repo. By default Omnibus stores packages in the project folder
itself: running the task multiple times would recursively add those artifacts to
the source files for the `datadog-agent` software definition.

## Windows Docker image (Windows host only, recommended)

### Prerequisites
To build on Windows, [Docker Desktop](https://docs.docker.com/docker-for-windows/install/) must be installed and configured to use Windows containers.

Start a Powershell prompt and navigate to your local clone of the `datadog-agent` repo.

 Run the following command:

```powershell
docker run -v "$(Get-Location):c:\mnt" -e OMNIBUS_TARGET=main -e MAJOR_VERSION=7 -e TARGET_ARCH=x64 datadog/agent-buildimages-windows_x64:1809 powershell -C "c:\mnt\tasks\winbuildscripts\Build-AgentPackages.ps1 -BuildOutOfSource 1 -InstallDeps 1 -CheckGoVersion 1"
```

Downloading the Docker image may take some time in the first run.

Alternatively here's a small Powershell script to facilitate using the docker image:
```powershell
param (
   [int]$MAJOR_VERSION=7,
   $TARGET_ARCH="x64",
   [bool]$RM_CONTAINER=$true,
   [bool]$DEBUG=$false
)

$cmd = "docker run"
if ($RM_CONTAINER) {
    $cmd += " --rm "
}
$opts = "-e OMNIBUS_TARGET=main -e MAJOR_VERSION=$MAJOR_VERSION -e TARGET_ARCH=$TARGET_ARCH"
if ($DEBUG) {
    $opts += " -e DEBUG_CUSTOMACTION=yes "
}
$cmd += " -m 8192M -v ""$(Get-Location):c:\mnt"" $opts datadog/agent-buildimages-windows_x64:1809 powershell -C ""c:\mnt\tasks\winbuildscripts\Build-AgentPackages.ps1 -BuildOutOfSource 1 -InstallDeps 1 -CheckGoVersion 1"""
Write-Host $cmd
Invoke-Expression -Command $cmd
```

If the build succeeds, the build artifacts can be found under `omnibus\pkg` in the repo.
