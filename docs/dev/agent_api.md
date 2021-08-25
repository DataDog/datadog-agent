# IPC (Inter Process Communication) API

The agent communicates with the outside world through an HTTP API to ease the
development of 3rd party tools and interfaces. The API is available from `localhost`
and through HTTPS only. It listens on port `5001` by default but can be configured differently.

## Security and Authentication

To avoid unprivileged users to access the API, authentication is required and based on a token.
The token is written to a file that's only readable by the user that the Agent runs as.

## Endpoints

Please refer to the [`cmd/agent/api`](https://github.com/DataDog/datadog-agent/tree/main/cmd/agent/api)
package for a list of endpoints implemented so far.

TODO: generate a list of endpoints with [swagger](http://swagger.io/)
