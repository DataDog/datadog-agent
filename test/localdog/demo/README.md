# localdog demo

This directory runs a Datadog Agent in Docker (`localdog-agent`) and a sample Node.js
service (`demo-node`). Together they send traces, logs and metrics to localdog on
`http://localhost:8282`.

```bash
./run-agent.sh   # start/restart the agent container (LOCALDOG_URL, LOG_DIR, NETWORK_MODE...)
./run-demo.sh    # start demo-node in the background (load generator on)
./stop-demo.sh
./stop-agent.sh
```

See [node-app/README.md](node-app/README.md) for what the app emits, the
configuration knobs and the known caveats.
