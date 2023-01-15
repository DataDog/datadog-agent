#ifdef __BALOUM__

#ifndef _BALOUM_H__
#define _BALOUM_H__

struct baloum_ctx
{
    __u64 arg0;
    __u64 arg1;
    __u64 arg2;
    __u64 arg3;
    __u64 arg4;
};
static void *(*baloum_malloc)(__u32 size) = (void *)0xffff;
static int (*baloum_call)(struct baloum_ctx *ctx, const char *section) = (void *)0xfffe;
static int (*baloum_strcmp)(const char *s1, const char *s2) = (void *)0xfffd;
static int (*baloum_memcmp)(const void *b1, const void *b2, __u32 size) = (void *)0xfffc;
static int (*baloum_sleep)(__u64 ns) = (void *)0xfffb;

#define assert_memcmp(b1, b2, s, msg)                                \
    if (baloum_memcmp(b1, b2, s) != 0)                               \
    {                                                                \
        bpf_printk("assert line %d : b1 != b2 : %s", __LINE__, msg); \
        return -1;                                                   \
    }

#define assert_strcmp(s1, s2, msg)                                   \
    if (baloum_strcmp(s1, s2) != 0)                                  \
    {                                                                \
        bpf_printk("assert line %d : s1 != s2 : %s", __LINE__, msg); \
        return -1;                                                   \
    }

#define assert_equals(v1, v2, msg)                                   \
    if (v1 != v2)                                                    \
    {                                                                \
        bpf_printk("assert line %d : v1 != v2 : %s", __LINE__, msg); \
        return -1;                                                   \
    }

#define assert_zero(v1, msg)                                           \
    if (v1 != 0)                                                       \
    {                                                                  \
        bpf_printk("assert line %d : v1 == 0 : %s", __LINE__, msg);    \
        return -1;                                                     \
    }

#define assert_not_zero(v1, msg)                                       \
    if (v1 == 0)                                                       \
    {                                                                  \
        bpf_printk("assert line %d : v1 != 0 : %s", __LINE__, msg);    \
        return -1;                                                     \
    }

#define assert_not_equals(v1, v2, msg)                               \
    if (v1 == v2)                                                    \
    {                                                                \
        bpf_printk("assert line %d : v1 == v2 : %s", __LINE__, msg); \
        return -1;                                                   \
    }

#define assert_not_null(v1, msg)                                       \
    if (v1 == NULL)                                                    \
    {                                                                  \
        bpf_printk("assert line %d : v1 == NULL : %s", __LINE__, msg); \
        return -1;                                                     \
    }

#define assert_null(v1, msg)                                           \
    if (v1 != NULL)                                                    \
    {                                                                  \
        bpf_printk("assert line %d : v1 != NULL : %s", __LINE__, msg); \
        return -1;                                                     \
    }

#endif

#endif