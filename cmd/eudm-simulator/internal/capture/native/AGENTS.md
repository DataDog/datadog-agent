# Native capture

Keep native collectors behind Windows/macOS build constraints. Portable replay
must never initialize this collector graph.

Initialize Agent feature detection before the shared process/container provider;
its collectors consult the feature registry during construction. Configure an
in-memory recording transport before starting any collection, and keep native
diagnostic logging disabled because OS errors can contain real user paths.

Count distinct collection cycles for coverage and cadence. Multiple encoded
chunks from one process collection share an offset and count as one cycle.

Disable demultiplexer service-check, event, and sketch payloads before creating
its serializers: those outputs do not pass through the capture transformer.
Use a placeholder demultiplexer hostname as an additional safeguard, and cover
implicit Agent output in privacy tests as well as explicitly submitted samples.
