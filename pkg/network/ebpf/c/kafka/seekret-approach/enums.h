#pragma once

enum message_type_t {
    kUnknown,
    kRequest,
    kResponse
};

enum traffic_direction_t {
    kEgress,
    kIngress,
};

enum traffic_protocol_t {
    kProtocolUnknown = 0,
    kProtocolHTTP,
    kProtocolHTTP2,
    kProtocolMySQL,
    kProtocolCQL,
    kProtocolPGSQL,
    kProtocolDNS,
    kProtocolRedis,
    kProtocolNATS,
    kProtocolMongo,
    kProtocolKafka,
    kNumProtocols
};

enum endpoint_role_t {
    kRoleClient = 1 << 0,
    kRoleServer = 1 << 1,
    kRoleUnknown = 1 << 2,
};

enum control_value_index_t {
    kTargetTGIDIndex = 0,
    kSeekretTGIDIndex,
    kNumControlValues,
};

enum target_tgid_match_result_t {
  TARGET_TGID_UNSPECIFIED,
  TARGET_TGID_ALL,
  TARGET_TGID_MATCHED,
  TARGET_TGID_UNMATCHED,
};

// All types of http2 frames exist in the protocol.
// Checkout https://datatracker.ietf.org/doc/html/rfc7540 under "Frame Type Registry" section.
enum frame_type_t {
    kDataFrame          = 0,
    kHeadersFrame       = 1,
    kPriorityFrame      = 2,
    kRSTStreamFrame     = 3,
    kSettingsFrame      = 4,
    kPushPromiseFrame   = 5,
    kPingFrame          = 6,
    kGoAwayFrame        = 7,
    kWindowUpdateFrame  = 8,
    kContinuationFrame  = 9,
};
