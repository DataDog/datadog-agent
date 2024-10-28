## package `serializer`

The Serializer is in charge of routing payloads from the different parts of the
agent to the Forwarder. The backend currently offers 2 versions of the intake
API. The V1 version takes JSON payloads, the V2 take protocol buffer payloads or
JSON depending on endpoint.

While moving all the existing endpoints to the new format, the agent will
support both API. That is why the serializer is here to choose a serialization
protocol depending on the content and use the correct Forwarder method.

To be sent, a payload needs to implement the **Marshaler** interface.
