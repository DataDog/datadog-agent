#ifndef _STRUCTS_USER_SESSIONS_H_
#define _STRUCTS_USER_SESSIONS_H_

struct user_session_t {
    u8 session_type;
    char data[246];
    u8 padding[9];
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
