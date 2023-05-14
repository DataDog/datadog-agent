# Cluster check package

This package holds the cluster-agent logic to detect and dispatch cluster checks
on node-agents that report to it. The actual scheduling as delegated to the
node-agents' collector, the cluster-agent is only responsible for spreading
the load across available nodes.


## Architecture

```
                               +-------------+
                               | node agents |
                               +-+-----------+
                                 |
                                 | queries (/v1/api/clusterchecks/)
                                 |
                       +---------v----------+
+----------+  setups   |       Handler      |   watches   +----------------+
| AutoConf <-----------+     Public API     +-------------> leaderelection |
+-----+----+           |  init logic, glue  |             +----------------+
      |                +---------+----------+
      |                          |
      | sends                    | inits
      | configs                  | passes queries
      |                          |
      |            +-------------v-----------+
      |            |        dispatcher       |
      +------------>   Runs the dispatching  |
                   |   logic in a goroutine  |
                   |                         |
                   |   +-----------------+   |
                   |   |  clusterStore   |   |
                   |   | holds the state |   |
                   +---+-----------------+---+
```

### Handler

The `Handler` class holds the init logic and the glue between components. It has the following
scope:

  - initialise the other classes and register them
  - handle the api calls from the node-agents, through the dispatcher
  - watch the leader-election status when configured, and handle the dispatcher's lifecycle accordingly

### dispatcher

The `dispatcher` has the following scope:

  - handle incoming messages (new / removed configs) from the AutoConf system, and
update the store accordingly
  - watch node statuses and de-register stale nodes
  - re-dispatch orphaned configs
  - expose its state to the Handler

### clusterStore and nodeStore

These classes hold the dispatching state and provide convenience methods to make sure the
state stays consistent.
To ensure `dispatcher` operations (that require several reads / writes to the store) are
atomic, the stores are designed with an external locking, held by the `dispatcher` on access.

## Node-agent communication

The node-agent queries the cluster-agent through the autodiscovery `ClusterChecksConfigProvider`.
As nodes can be removed without notice, the cluster-agent has to detect when a node is not
connected anymore and re-dispatch the configurations to other, active, nodes.

This is handled by the `node_expiration_timeout` option (30 seconds by default) and the
`dispatcher.expireNodes` method. The node-agents heartbeat is updated when they POST on the
`status` url (10 seconds in the default configuration). When that heartbeat timestamp is too
old, the node is deleted and its configurations put back in the dangling map.
