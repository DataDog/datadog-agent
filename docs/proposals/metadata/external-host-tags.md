# External Host Tags collection

- Authors: Massimiliano Pippi
- Date: 2018-01-15
- Status: accepted
- [Discussion](https://github.com/DataDog/datadog-agent/pull/1053)

## Overview

Some checks (at this moment there are only 2 of them: [vsphere][] and [openstack][])
expose a method with the signature `get_external_host_tags()` that's called by
the Agent whenever it's time to build and send the metadata payload. The way the
new Agent collects metadata has drastically changed and this feature wasn't been
ported yet, thus such checks are currently not working.

## Problem

Previous versions of the Agent took advantage of the dynamic nature of Python
and invoked the collection method [like this][agent5-collection], an alternative
must be defined to provide the new Agent with the same feature.

## Constraints

Changes to the checks must be compatible with both the agents 5.x and 6.x so that
we don't need to backport the changes to Agent 5.

## Recommended Solution

### Change the collection model from "Pull" to "Push"

Instead of invoking a method exposed by the checks at every metadata collection,
let the checks push data to the Agent.

To keep the Agent in charge of deciding when to send those payloads to the intake
(such payloads might be hard to ingest so we need control), External tags would
be cached and used by the Agent when it's time to send the metadata payload.
External host tags coming from different checks could be sent in the same payload.

This would only require to add a new method to the `Sender` exposed to Python checks
by the Agent. On the check side, `get_external_host_tags` would be preserved for
backward compatibility and some logic would be added so that that method is called
from the `check` method but only in Agent6; Agent5 will keep the current behaviour,
dynamically calling by itself `get_external_host_tags` outside the collection cycle.

If collecting those tags is heavy and we can't afford to do it at every collection
cycle, some strategy would be needed (only when the check runs in Agent6) to do
that every few iterations instead: this is out of the scope of this proposal and
should be handled on a case by case basis.

- Strengths
    - keep the Agent code simple and move logic to the check
- Weaknesses
    - collection model would change completely and some issue might be introduced

## Other Solutions

### Replicate 5.x behaviour

To replicate the same behaviour as in Agent 5, where it's the Agent
[invoking a method][agent5-collection] on the check, the following logic should
be added to the metadata collection:

```
  for every check instance currently running {
    call the `get_external_host_tags`
  }
```

This can be achieved in two ways:

 * add a `GetExternalHostTags` method to the `Check` interface
 * type assert each instance to `py.Check`, call `getattr` on the underlying
   Python class and eventually call `get_external_host_tags`

- Strenghts
  - keep the same strategy with a low chance of breaking things
- Weaknesses
  - adding a method to the `Check` interface that would be actually implemented
    on 2 out of ~80 checks seems silly
  - the runtime machinery to call `getattr` is an epic hack that opens to
    possible debugging nightmares


[openstack]: https://github.com/DataDog/integrations-core/blob/a621eacb60e825cf9fd1f7cd6b18312c3ee103a6/openstack/datadog_checks/openstack/openstack.py#L938
[vsphere]: https://github.com/DataDog/integrations-core/blob/a621eacb60e825cf9fd1f7cd6b18312c3ee103a6/vsphere/datadog_checks/vsphere/vsphere.py#L522
[agent5-collection]: https://github.com/DataDog/dd-agent/blob/54922f56e386dc452ce1eae3b4e054237fd74ace/checks/collector.py#L718
