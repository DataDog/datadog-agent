# Tailers

Tailers are responsible for gathering log messages and sending them for further handling by the remainder of the logs agent.
It is the responsibility of a launcher to create and manage tailers.

A tailer sends log messages via a `chan *message.Message`.
