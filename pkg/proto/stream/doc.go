package stream

// The stream package implements "streaming" encoding of protobuf messages.
//
// While protobuf is not intended for use in infinite streams, such an
// interface is still useful for rapidly creating protobuf messages from data
// that is not already in easily-marshaled structs.  In other words, this
// package can save a substantial amount of copying and allocation over an
// equivalent implementation using more typical struct marshaling.
//
// The drawback is that the implementation requires a more detailed
// understanding of the protobuf encoding, including the field numbers and
// types for the messages being encoded.
//
// ## Usage
//
// Begin by creating a new `ProtoStream` with `New`, passing an `io.Writer`
// to which the output bytes should be written.  In many cases this is simply
// a `bytes.Buffer`, but any writer will do.
//
// Next, for each field, call the method appropriate to its protobuf type,
// passing the field number as the first argument.  For repeated fields
// containing scalar types, use the `*Packed` methods to write the packed form.
// For non-scalar repeated fields, call the encoding method repeatedly.
//
// Note that most methods will do nothing when given their zero value.
