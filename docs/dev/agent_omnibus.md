# Build the Agent packages

Agent packages for all the supported platforms are built using
[Omnibus](https://github.com/chef/omnibus).

## Prepare the dev environment

To run Omnibus you need the following:

 * Ruby 2.2 or later
 * Bundler

Omnibus will be invoked through `invoke` and most of the details will be handled
by the specific tasks for the Agent. Unless you [use Docker][#docker] to build
the packages, Omnibus will use the specific format for the local operating system,
e.g. you'll get a Deb package on Debian-based distros, an RPM package on RedHat
based distros, an MSI installer on Windows, a `.pkg` package bundled in a DMG
archive on Mac.

### Linux and Mac

The project will be built locally then compressed in the final deb/rpm/dmg artifact.
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
inv agent.omnibus-build --base-dir=$HOME/.omnibus
```

On Mac, you might want to skip the signing step by running:
```
inv agent.omnibus-build --base-dir=$HOME/.omnibus --skip-sign
```

The path you pass with the `--base-dir` option will contain the sources
downloaded by Omnibus in the `src` folder, the binaries cached after building
those sources in the `cache` folder and the final deb/rpm/dmg artifacts in the
`pkg` folder. You can fine tune an Omnibus run passing more options, see
`inv agent.omnibus-build --help` for the list of all the available options.

**Note:** it's strongly advised to pass `--base-dir` and point to a directory
outside the Agent repo. By default Omnibus stores packages in the project folder
itself: running the task multiple times would recursively add those artifacts to
the source files for the `datadog-agent` software definition.

### Docker

Let's see in detail how to use Docker to build a Debian package, you need to run
Docker on your local machine before moving on.

Clone the repo containing the Dockerfiles Datadog uses to build the official
packages:
```
git clone https://github.com/DataDog/datadog-agent-buildimages && cd datadog-agent-buildimages
```

Depending on which kind of package you want, you should build the relevant image,
in this case, since we want a Deb package, we'll build `deb-x64`:
```
docker build -t datadog-agent-buildimages:deb_x64 -f deb-x64/Dockerfile .
```

From the Agent source folder, start a Docker container like this:
```
docker run -v "$PWD:/datadog-agent" -v "/tmp/omnibus:/omnibus" -v "/tmp/opt/datadog-agent:/opt/datadog-agent" -v"/tmp/gems:/gems" --workdir=/datadog-agent datadog-agent-buildimages:deb_x64 inv -e agent.omnibus-build --base-dir=/omnibus --gem-path=/gems
```

The container will share 3 volumes with the host to avoid starting from scratch
at each Omnibus run:

 * `/tmp/omnibus`, containing the Omnibus base dir
 * `/tmp/opt/datadog-agent`, containing the Omnibus installation dir
 * `/tmp/gems`, containing all the ruby gems installed with Bundler

The builder images are also available in DocherHub. You can use them instead of
building your own by using `datadog/agent-buildimages-rpm_x64` or `datadog/agent-buildimages-deb_x64`.

If the build images crash when you run them on modern Linux distributions, you might be 
affected by [this bug](https://github.com/moby/moby/issues/28705).

### Windows

TODO.
