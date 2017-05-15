## package `metadata`

This package is responsible to provide metadata in the right form to be directly sent to the backend.
Single metadata providers are defined in the form of insulated sub packages that should expose a public
method like:
```go
func GetPayload() *Payload
```
along with the specific `Payload` definition.

For the time being, any subpackage provides a piece of information that is used in the `v5` package to
compose a single metadata payload compatible with the one from Agent v.5. This way we can send
metadata through the current backend endpoints, waiting for the new ones to be deployed. At that point,
all the subpackages will be required to define their payloads with the new Protobuf format.

Please keep this README updated.