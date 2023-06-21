# JMX Checks Development
[JMXFetch](https://github.com/DataDog/jmxfetch/) is the component of the Agent which is responsible with collecting
metrics from Java applications.

These docs are intended to help you test changes to either JMXFetch or JMX-based
integrations locally. Ie, you as a developer have made some changes to how JMXFetch
works and you'd like to run JMXFetch via the Agent to validate your changes.

## Custom Build of JMXFetch
1. [Build JMXFetch](https://github.com/DataDog/jmxfetch/#building-from-source)
2. Copy the resulting jar into `$GOPATH/src/github.com/DataDog/datadog-agent/dev/dist/jmx/jmxfetch.jar`
3. Run `inv agent.run`
4. Validate you see the JMXFetch section in `agent status`

If you have a jmx-based integration configured to run, it should automatically
be run in your local JMXFetch instance.

## Custom JMX-based Check
1. Run your JMX server listening on `localhost`
    1. A testing JMX server can be found
    [here](https://github.com/DataDog/jmxfetch/tree/master/tools/misbehaving-jmx-server)
2. Add a check configuration that specifies how to connect to your JMX server
    1. An example for the above test server can be found
    [here](https://github.com/DataDog/jmxfetch/blob/master/tools/misbehaving-jmx-server/misbehaving-jmxfetch-conf.yaml)
    2. This config should live in `dev/dist/conf.d/jmx-test-server.d/conf.yaml`
3. Run `inv agent.run`
4. Validate you see the check scheduled in `agent status`
