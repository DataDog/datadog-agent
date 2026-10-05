package wire

import "github.com/google/uuid"

// [MS-DTYP] 2.3.4 defines a GUID as the structure
//
//	typedef struct _GUID {
//	    unsigned long  Data1;
//	    unsigned short Data2;
//	    unsigned short Data3;
//	    unsigned char  Data4[8];
//	} GUID;
//
// Data1, Data2 and Data3 are little-endian on the wire, whereas uuid.UUID
// stores every field in the big-endian byte order of [RFC 9562]. The helpers
// below convert between the two representations.

// encodeGUID writes u to b (16 bytes) in the on-wire Microsoft GUID layout.
func encodeGUID(u uuid.UUID, b []byte) {
	b[0], b[1], b[2], b[3] = u[3], u[2], u[1], u[0]
	b[4], b[5] = u[5], u[4]
	b[6], b[7] = u[7], u[6]
	copy(b[8:16], u[8:])
}

// decodeGUID reads a Microsoft GUID from b (16 bytes) into canonical order.
func decodeGUID(b []byte) uuid.UUID {
	var u uuid.UUID
	u[0], u[1], u[2], u[3] = b[3], b[2], b[1], b[0]
	u[4], u[5] = b[5], b[4]
	u[6], u[7] = b[7], b[6]
	copy(u[8:], b[8:16])
	return u
}
