# Prototype Schema

**schema.proto**

Define the shape of the schema's protobuf version.

Run the protoc compiler on this to get `gen/schema.pb.go` and `gen/schema_pb2.py`

**fields.message**

An instance of the schema.proto message. Serialize this to binary to get `gen/fields.bin`

**fields.bin**

A binary compiled version of the agent schema. Loaded in `pkg/config/setup/config.go` in order to build the in-memory agent schema.

This file gets embedded into the agent using `go:embed` and then iterated over when building the schema.

Ideally, this file *would not* contain the names of settings. The names themselves take up a lot of space, and their data already exists in the agent binary, so by having them in this file they appear twice. It would be better if we could "intern" these names into a golang source file, for example by putting them into a big static array. Then this fields.bin file could use indicies into this array, and the golang linker would combine strings in that static array with the strings section of the agent binary.

**compiler.py**

Simple version of the processor written in python. Here we just compile fields.message into fields.bin. The real version would use a better frontend and do real validation of the input.