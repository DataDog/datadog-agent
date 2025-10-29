# RFC: Kubernetes Actions via Remote Configuration

## Problem Statement

We need a mechanism to execute one-time administrative actions on Kubernetes resources in customer clusters, initiated from the Datadog UI. These actions include operations like deleting pods, restarting deployments, draining nodes, and other cluster management tasks. The actions should be:

- Triggered by users through the Datadog UI
- Validated for authorization before execution
- Executed exactly once (no duplicate execution)
- Trackable for completion status

The proposed solution will push validated actions to remote configuration, where they can be subscribed to by the cluster agent for execution.

## High-Level Architecture

```
User (UI)
  ↓
Backend Validation Service (Authorization + Validation)
  ↓
Remote Configuration (Action Distribution)
  ↓
Cluster Agent (Action Execution)
  ↓
Status Reporting Service (Optional - for completion tracking)
```

**Key Components:**

1. **UI**: User initiates action (e.g., "delete pod xyz")
2. **Backend Validation Service**: Validates user permissions and action safety
3. **Remote Configuration**: Distributes validated actions to cluster agents
4. **Cluster Agent**: Executes actions and tracks execution history
5. **Status Reporting Service** (future): Receives execution results from agents

## Authorization & Validation

The **backend validation service** is responsible for:

- **Access Control**: Determining if a user has the correct permissions to perform an action on a specific resource
- **Action Validation**: Ensuring the action is safe and permitted (e.g., preventing deletion of critical system pods)
- **Rate Limiting**: Preventing abuse through excessive action requests
- **Audit Logging**: Recording all action requests for compliance

Only actions that pass validation will be pushed to remote configuration for execution.

## Proposal

### RC Schema

We need to define a schema for distributing one-time Kubernetes actions from the backend to the cluster agent. The schema must support:

- Generic action types (extensible for future actions)
- Targeting any Kubernetes resource (pods, deployments, nodes, etc.)
- Additional parameters for action-specific configuration
- Unique identification to prevent duplicate execution

#### Proposed Schema

The schema will be introduced in:
- `agent-payload`: [agent-payload/proto/kubeactions](https://github.com/DataDog/agent-payload/tree/master/proto/kubeactions)
- `rc-schema-validation`: [dd-go/remote-config/apps/rc-schema-validation/schemas/kubeactions.json](https://github.com/DataDog/dd-go/tree/prod/remote-config/apps/rc-schema-validation/schemas)

```json
{
  "actions": [
    {
      "action_type": "delete_pod",
      "resource": {
        "api_version": "v1",
        "kind": "Pod",
        "namespace": "default",
        "name": "my-pod-xyz"
      },
      "parameters": {
        "grace_period_seconds": "30"
      },
      "timestamp": {
        "seconds": 1234567890,
        "nanos": 0
      }
    }
  ]
}
```

**Schema Fields:**

- `action_type`: String identifier for the action (e.g., "delete_pod", "restart_deployment", "drain_node")
- `resource`: Kubernetes resource specification
  - `api_version`: Resource API version (e.g., "v1", "apps/v1")
  - `kind`: Resource kind (e.g., "Pod", "Deployment", "Node")
  - `namespace`: Namespace (optional for cluster-scoped resources)
  - `name`: Resource name
- `parameters`: Map of action-specific parameters (optional)
- `timestamp`: When the action was created

### Action Execution & Duplicate Prevention

**Critical Requirement:** Actions must execute exactly once, even across agent restarts, to prevent unintended duplicate operations.

**Implementation:**

Each action in remote configuration has a unique `metadata.id` and `metadata.version` (provided by RC). The cluster agent:

1. **Tracks Executed Actions**: Maintains a persistent store of executed action IDs
2. **Checks Before Execution**: Before executing, checks if the action ID has already been processed
3. **Marks As Executed**: After execution (success or failure), records the action ID with status
4. **Survives Restarts**: Persistent storage ensures tracking survives agent restarts

**Storage Options:**

This is probably the biggest question mark right now. Options for persistent storage would include
creating a config map on the cluster, storing the info in etcd, or storing the info serverside if we're
unable to store it locally - with the obvious downside of having to make a network call before
each execution. // TODO: where else in the agent do we store state?

### Status Reporting

**Challenge:** Remote configuration callbacks do not support rich execution results.

When we acknowledge a config in RC, we can only report success/failure, not detailed execution results. For tracking completion status of actions:

**Status Reporting Service (Recommended for Future)**

The cluster agent would POST execution results to a dedicated status reporting service:

```json
POST /api/v1/kubeactions/status
{
  "org_id": "123",
  "cluster_id": "abc",
  "action_id": "action-123",
  "version": 1,
  "status": "success",
  "message": "pod deleted successfully",
  "executed_at": "2024-10-24T12:00:00Z"
}
```

### Agent-Side Concerns

**Security:**

- Need robust validation framework with configurable safety rules
- RBAC permissions for cluster agent service account must be carefully scoped

**Performance:**

- If some form of afforementioned local storage was implemented we need to think about storage limits

**Reliability:**

- What happens if action fails mid-execution?
- Consider retry logic for transient failures (with safeguards against infinite retries)

### Alternative: Dedicated Action Service

**Important Consideration:** Remote configuration is designed for configuration management, not one-time action execution. Using it for actions has limitations:

1. **Not Designed for Actions**: RC is optimized for eventual consistency of configurations, not reliable delivery of one-time commands
2. **No Native Ordering**: No guarantee of action execution order
3. **Limited Feedback**: Cannot easily report detailed execution results
4. **Retention Issues**: Old actions accumulate in RC if not cleaned up properly

**Recommended Future Direction:**

The Remote Configuration team is developing a dedicated service for one-time actions/commands: [RC Actions Service Design Doc](https://docs.google.com/document/d/1_SehBKvNTFYIre-zEZ_21zU1GLGTrxlCFU67wYWB28w/edit?tab=t.4go3q8nt4z2w#heading=h.kh490egl0i0v)

This service would provide:
- Reliable message delivery semantics
- Built-in idempotency handling
- Rich status reporting
- Action queuing and ordering
- Better tooling for action management

**Recommendation:** Implement initial version with RC, but closely monitor the progress of a proper action service and plan
for migration


### RC API Internal

Endpoints will be defined in: `dd-go/pb/proto/remote-config/api-internal/kubeactions/`

#### PublishAction(orgId, clusterId, action)

Given an `orgId`, `clusterId`, and a validated action payload, add a new action to remote config for the specified cluster. Each action receives a unique `metadata.id` and `metadata.version` from RC.

This endpoint will be called by the backend validation service after a user action passes authorization and validation checks.

**Note:** Actions are one-time operations, so they should be cleaned up after execution. Consider implementing automatic expiration (e.g., 24 hours after execution).

#### DeleteActions(orgId, clusterId, actionIds)

Given an `orgId`, `clusterId`, and optional list of `actionIds`, delete specified actions (or all actions if no IDs provided) from RC for the cluster.

#### ListActions(orgId, clusterId)

Given an `orgId` and `clusterId`, return a list of pending actions in RC. This is primarily for debugging and monitoring.

#### UpdateAction
Should not support action updating as this could lead to confusion and inconsistency

### Status Reporting API (Future)

Endpoints will be defined in: `backend/api/kubeactions/status/`

#### ReportStatus(orgId, clusterId, actionId, version, status, message)

Allows cluster agent to report execution results. The backend can then:
- Update UI to show action completion
- Trigger alerts on failures
- Store audit trail of all actions

## Initial Action Types

**POC Actions:**

- `delete_pod`: Delete a pod
- `restart_deployment`: Restart a deployment by updating restart annotation

**Possible Future Actions:**

- `scale_deployment`: Scale deployment replicas
- `cordon_node`: Mark node as unschedulable
- `uncordon_node`: Mark node as schedulable
- `drain_node`: Safely evict pods from a node
- `patch_resource`: Generic patch operation
- `rollback_deployment`: Rollback to previous revision
- `delete_job`: Delete a job and its pods
- `trigger_cronjob`: Manually trigger a cronjob
- Custom user-defined actions via webhooks

## Security Considerations

1. **Agent Side:**
   - Service account must have minimal required RBAC permissions
   - Implement configurable action allowlist/blocklist
   - Add safety checks (e.g., prevent deleting kube-system pods)
   - Consider namespace-based restrictions

2. **Backend Side:** (Will have separate RFC)
   - Strong authorization
   - Rate limiting per user/org
   - Audit logging of all actions
   - Validation of resource existence before publishing

## Open Questions

1. **Should we implement status reporting in the initial version?**

2. **How long should we retain executed action history in local storage?**

3. **Should we support bulk actions (multiple resources in one action)?**
