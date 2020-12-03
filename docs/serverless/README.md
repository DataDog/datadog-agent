# Serverless Agent

The Serverless Agent is a build of the Datadog Agent capable of running
in serverless environments, to collect custom metrics, function's logs and soon
function's traces. Today, its main usage is to build the Datadog Lambda
Extension for AWS Lambda.

## Concepts

Today, the Serverless Agent is intended to run as a side-car of the serverless function.

### Validate the presence of the Serverless Agent and order a flush

The Serverless Agent is running an HTTP server on port 8124.

With the Datadog Lambda Extension, this HTTP servers is listening for two
different routes:
    - /lambda/hello
    - /lambda/flush

These routes are not doing anything in particular while running in AWS, however,
they can be at some point and if the Serverless Agent has to support a new serverless
environment one day, it would be important to create different HTTP routes.

#### `Hello` route

No payload.

This is a discover route: by calling this HTTP route, a function can know if the
Serverless Agent is running.


#### `Flush`

No payload.

This route is used to tell the Serverless Agent that it has to immediately flush
the data it has buffered. This route is blocking and replying only once all the
data has been flushed to the Datadog intake.

### Configuration

The configuration of the Serverless Agent goes through the regular `config` package
of the Datadog Agent, reading both in a file and in the environment variables.

For the Datadog Lambda Extension, some fields are set directly in the environment
at startup for the config package to pick them up. (See `cmd/serverless/main.go` for the
startup process).

