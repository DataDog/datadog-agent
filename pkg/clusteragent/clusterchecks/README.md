# Cluster check package

This package holds the cluser-agent logic to detect and dispatch cluster checks
on node-agents that report to it. The actual scheduling as delegated to the
node-agents' collector, the cluster-agent is only responsible for spreading
the load across available nodes.



## Architecture

```
                                  +-------------+
                                  | node-agents |
                                  +-+-----------+
                                    | queries (/v1/api/clusterchecks/)
                                    |
   +----------+                     |
   | AutoConf < - - - - - - - \     |
   +-----+----+        setups |     |
         |                    |     |
         |                    |     v          watches   +----------------+
         |                    \-  Handler  --------------> leaderelection |
   sends |                      Public API               +----------------+
 configs |                   init logic, glue.
         |                          |
         |                          |
         |                          |
         |                          |
         |                          | init
         v                          | reads
     dispatcher               init  | writes          clusterStore
Runs the dispatching  < - - - - - - +------------>  holds the state
logic in a goroutine                                       ^
         |                                                 |
         |                   reads, writes                 |
         \-------------------------------------------------/

```

### Handler

The `Handler` class holds the init logic and the glue between components. It has the following
scope:

  - initialise the other classes and register them
  - handle the api calls from the node-agents, read configs and write node statuses
  - watch the leader-election status when configured, and handle the dispatcher's lifecycle accordingly

### dispatcher

The `dispatcher` has the following scope:

  - handle incoming messages (new / removed configs) from the AutoConf system, and
update the store accordingly
  - watch node statuses and de-register stale nodes
  - re-dispatch orphaned configs

### clusterStore and nodeStore

These classes hold the dispatching state and provide convenience methods to make sure the
state stays consistent.
To ensure `dispatcher` operations (that require several reads / writes to the store) are
atomic, the stores are designed with an external locking, held by the `Handler` and
`dispatcher` on access.

## Node-agent communication

The node-agent queries the cluster-agent through the autodiscovery `ClusterChecksConfigProvider`.


