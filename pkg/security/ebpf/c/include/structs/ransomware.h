#ifndef __STRUCTS_RANSOMWARE_H__
#define __STRUCTS_RANSOMWARE_H__

struct ransomware_score_t {
    u64 first_syscall;
    u64 last_syscall;

    u32 new_file;
    u32 unlink;
    u32 rename;
    u32 urandom;
    u32 kill;

    u32 already_notified;
};

#endif // __STRUCTS_RANSOMWARE_H__
