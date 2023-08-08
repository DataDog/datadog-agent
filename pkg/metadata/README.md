## package `metadata`

This package is responsible to provide metadata in the right form to be directly sent to the backend.
Metadata collection is iterated during Agent execution at different time intervals for different
use cases.

### Providers
Single metadata providers are defined in the form of insulated sub packages exposing a public
method like:
```go
func GetPayload() *Payload
```
along with their specific `Payload` definition. Payload formats can be different, that's why metadata
providers are not implemented as interfaces. These components should be loosely coupled with the rest
of the Agent, this way they can be used as independent go packages in different projects and different
environments.

### Collectors
Collectors are used by the Agent and are supposed to be run periodically. They are
responsible to invoke the relevant `Provider`, collect all the info needed, fill the appropriate
payload and send it to the specific endpoint in the intake. Collectors are allowed to be strongly coupled
to the rest of the Agent components because they're not supposed to be used elsewhere.
Collectors can be user configurable, except for the `host` metadata collector that is always scheduled
with a default interval.

**Notice:** For the time being, several providers collect a piece of information that is used in
the `v5` package to compose a single metadata payload compatible with the one from Agent v.5.
This way we can send metadata through the current backend endpoints
(see `HostCollector`), waiting for the new ones to be deployed.
At that point, all the subpackages will be required to define a payload with either the new Protobuf format
or a custom JSON compatible with the v2 intake API.
