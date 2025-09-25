# JMXFetch
[JMXFetch](https://github.com/DataDog/jmxfetch/) is the component of the Agent which is responsible for collecting metrics from Java applications.

For more details on JMXFetch or developer documentation, please visit the project documentation on the [JMXFetch GitHub repo](https://github.com/DataDog/jmxfetch/).

## Running checks requiring JMXFetch
If your goal is to run a JMX-based check:

1. Download the `-jar-with-dependencies.jar` build of the latest version of JMXFetch from
   [`maven`](https://repo1.maven.org/maven2/com/datadoghq/jmxfetch/), or [create a custom build](https://github.com/DataDog/jmxfetch/#building-from-source).
1. Copy the jar file and rename it to `$GOPATH/src/github.com/DataDog/datadog-agent/dev/dist/jmx/jmxfetch.jar`.
1. Run `dda inv agent.run`.
1. Validate that the JMXFetch section appears in `agent status`.

If you have a JMX-based integration configured to run, it automatically runs in your local JMXFetch instance.
