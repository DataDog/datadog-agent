## package `fanout`

This package implements a channel-based 1-n communication, where one
source is listened to by several listeners. It allows listeners to
subscribe and unsuscribe, and handles the disconnection logic if one
listener falls behind.

or), then fail with a `PermaFail`

### How to use the Fanout package

You should not directly embed the MessageFanout class, but use it
as a template for autogenerating a package-local fanout class that
will handle the exact type you want to pass around.

### How to use a class embedding Fanout
