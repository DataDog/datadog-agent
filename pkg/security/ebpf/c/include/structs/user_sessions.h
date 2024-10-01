#ifndef _STRUCTS_USER_SESSIONS_H_
#define _STRUCTS_USER_SESSIONS_H_

// 16 bytes of key + 1 byte of session_type + 239 bytes of data = 256 bytes, the size of the eRPC payload

struct user_session_t {
    u8 session_type;
    char data[239];
    u8 padding[16];
};

struct user_session_key_t {
    u64 id;
    u8 cursor;
    u8 padding[7];
};

struct user_session_request_t {
    struct user_session_key_t key;
    struct user_session_t session;
};

#endif
