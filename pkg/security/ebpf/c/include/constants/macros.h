#ifndef _CONSTANTS_MACROS_H
#define _CONSTANTS_MACROS_H

#define LOAD_CONSTANT(param, var) asm("%0 = " param " ll" \
                                      : "=r"(var))
#define IS_UNHANDLED_ERROR(retval) retval < 0 && retval != -EACCES &&retval != -EPERM
#define IS_ERR(ptr) ((unsigned long)(ptr) > (unsigned long)(-1000))
#define IS_KTHREAD(ppid, pid) ppid == 2 || pid == 2
#define NS_TO_SEC(x) (x) / 1000000000
#define SEC_TO_NS(x) (x) * 1000000000

#define PARSE_FUNC(STRUCT)                                                                                \
    __attribute__((always_inline)) struct STRUCT *parse_##STRUCT(struct cursor *c, struct STRUCT *dest) { \
        struct STRUCT *ret = c->pos;                                                                      \
        if (c->pos + sizeof(struct STRUCT) > c->end)                                                      \
            return 0;                                                                                     \
        c->pos += sizeof(struct STRUCT);                                                                  \
        *dest = *ret;                                                                                     \
        return ret;                                                                                       \
    }

#define DECLARE_EQUAL_TO_SUFFIXED(suffix, s)                                 \
    static __attribute__((always_inline)) int equal_to_##suffix(char *str) { \
        char s1[sizeof(#s)];                                                 \
        bpf_probe_read(&s1, sizeof(s1), str);                                \
        char s2[] = #s;                                                      \
        for (int i = 0; i < sizeof(s1); ++i)                                 \
            if (s2[i] != s1[i])                                              \
                return 0;                                                    \
        return 1;                                                            \
    }

#define DECLARE_EQUAL_TO(s) \
    DECLARE_EQUAL_TO_SUFFIXED(s, s)

#define IS_EQUAL_TO(str, s) equal_to_##s(str)

#endif
