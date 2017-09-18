# Build the Agent packages

Agent packages for all the supported platforms are built using
[Omnibus](https://github.com/chef/omnibus).

## Prepare the dev environment

To run Omnibus you need the following:

 * Ruby 2.2 or later
 * Bundler

Omnibus will be invoked through `invoke` and most of the details will be handled
by the specific tasks for the Agent.

### Linux and Mac

The project will be built locally in the same installation path of the final
package (`/opt/datadog-agent`) before being included and compressed in the
final rpm/deb/dmg artifact. This means that if you already have the Agent installed,
you might need to move it to a different location before operating Omnibus.

Create the directory that will contain the package files:
```
sudo mkdir /opt
```

If you want to run Omnibus as an unprivileged user (suggested unless you're going
to run Omnibus in a Docker container), the folder has to be world-writable:
```
sudo chmod 0777 /opt
```

From the Agent source folder, run the `invoke` task like this:
```
inv agent.omnibus-build --base-dir=$HOME/local
```

The path you pass with the `--base-dir` option will contain the sources
downloaded by Omnibus in the `src` folder, the binaries cached after building
those sources in the `cache` folder and the final deb/rpm/dmg artifacts in the
`pkg` folder.

### Windows

TODO.