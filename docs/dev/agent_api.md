# IPC (Inter Process Communication) API

The agent communicates with the outside world through an HTTP API to ease the
development of 3rd party tools and interfaces. By default, the API is available
at TCP port `5001` on `localhost` but can be configured differently.

## Security and Authentication

To avoid unprivileged users to access the API, authentication is required.

FIXME: new authentication system wasn't merged yet: https://github.com/DataDog/datadog-agent/pull/939

## Endpoints

Endpoints implemented so far (this list should be killed in favor of [swagger](http://swagger.io/)
at some point):
    * [GET] http://localhost/agent/version
