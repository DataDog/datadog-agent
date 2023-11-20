# JMX Checks Development
[JMXFetch](https://github.com/DataDog/jmxfetch/) is the component of the Agent which is responsible for collecting metrics from Java applications.

These docs are intended to help you test changes to either JMXFetch or JMX-based
integrations locally. In other words, you've made changes to how JMXFetch
works and you'd like to run JMXFetch via the Agent to validate your changes.

## Stable JMXFetch
If your goal is to run a JMX-based check and you don't need a custom
version of JMXFetch, follow the instructions below:

1. Download the `-jar-with-dependencies.jar` build of the latest version of JMXFetch from
   [`maven`](https://repo1.maven.org/maven2/com/datadoghq/jmxfetch/)
2. Copy the jar file and rename it to `$GOPATH/src/github.com/DataDog/datadog-agent/dev/dist/jmx/jmxfetch.jar`.
3. Run `inv agent.run`.
4. Validate that the JMXFetch section appears in `agent status`.

If you have a JMX-based integration configured to run, it automatically
runs in your local JMXFetch instance.


## Custom Build of JMXFetch
1. [Build JMXFetch](https://github.com/DataDog/jmxfetch/#building-from-source).
2. Copy the resulting jar into `$GOPATH/src/github.com/DataDog/datadog-agent/dev/dist/jmx/jmxfetch.jar`.
3. Run `inv agent.run`.
4. Validate that the JMXFetch section appears in `agent status`.

If you have a JMX-based integration configured to run, it should automatically
be run in your local JMXFetch instance.

## Custom JMX-based Check
1. Run your JMX server listening on `localhost`.
    1. A testing JMX server can be found
    [here](https://github.com/DataDog/jmxfetch/tree/master/tools/misbehaving-jmx-server).
2. Add a check configuration that specifies how to connect to your JMX server
   - An example for the above test server can be found
   [in the jmxfetch repo](https://github.com/DataDog/jmxfetch/blob/master/tools/misbehaving-jmx-server/misbehaving-jmxfetch-conf.yaml).
   - This config should live in `dev/dist/conf.d/jmx-test-server.d/conf.yaml`.
3. Run `inv agent.run`.
4. Validate that the check appears as scheduled in `agent status`.
