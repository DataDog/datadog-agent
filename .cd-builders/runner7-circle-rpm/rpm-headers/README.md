# systemd

To build the log-agent journald integration on Centos 7, we need to build the
agent with the systemd tag.
This will trigger the build of code that calls the `coreos/go-systemd` package.
To compile, this package requires some systemd headers to be present.

We collected the headers from systemd version 219 (the default systemd version
on Centos 7) and we put them on our Centos 6 based builder image. It works because
the binary doesn't expect systemd shared objects for dynamic linking.

Instead the systemd shared objects are loaded at runtime by the go code: ([see here](https://github.com/coreos/go-systemd/blob/c8cc474ba8655dfbdb0ac7fcc09b7faf5b643caf/sdjournal/functions.go#L46))

If the shared objects are not present on the computer, the `go-systemd` package
returns an error that we gracefully handle in the setup of a journald tailer.
