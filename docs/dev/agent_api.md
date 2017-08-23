# IPC (Inter Process Communication) API

The agent communicates with the outside world through an HTTP API to ease the
development of 3rd party tools and interfaces. Since HTTP is transported over
a [Unix Socket](https://en.wikipedia.org/wiki/Unix_domain_socket) on *nix platforms
and [Named Pipes](https://msdn.microsoft.com/en-us/library/windows/desktop/aa365590.aspx)
on Windows, authorization is delegated to the filesystem.

Endpoints implemented so far (this list should be killed in favor of [swagger](http://swagger.io/)
at some point):
    * [GET] http://localhost/agent/version
