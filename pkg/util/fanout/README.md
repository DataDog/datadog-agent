# package `fanout`

This package implements a channel-based 1-n communication, where one
source is listened to by several listeners. It allows listeners to
subscribe and unsuscribe, and handles the disconnection logic if one
listener falls behind.

## How to use the Fanout package

You should not directly embed the MessageFanout class, but use it
as a template for autogenerating a package-local fanout class that
will handle the exact type you want to pass around.

This is done by using `go generate` and `gorewrite`, see https://github.com/taylorchu/generic
for the `gorewrite` documentation and `pkg/util/docker`'s'
[GoRewrite.yaml](https://github.com/DataDog/datadog-agent/blob/master/pkg/util/docker/GoRewrite.yaml) and
[autogen.go](https://github.com/DataDog/datadog-agent/blob/master/pkg/util/docker/autogen.go) for usage
example.

Once your files are setup, call `inv gorewrite` to generate the code.

You can then either use the class standalone and manage its calls,
or embed it and let clients use its public interface.

## How to use a class embedding Fanout

Call `SuscribeChannel` to get two channels:
  - a data channel where data from the source will be streamed to you
  - an error channel, on which you can receive either:
     - `io.EOF` if the source closes the channel
     - `fanout.ErrTimeout` if you have been disconnected because your
     communication channel is full (you fell behind). It is your responsibility
     to restart from a clean state and re-suscribe.

A `SuscribeCallback` method will be implemented soon.
