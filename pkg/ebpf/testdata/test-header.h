#ifndef TEST_H
#define TEST_H

#include <linux/types.h>

#ifndef SOME_CONSTANT
#define SOME_CONSTANT 10
#endif

struct test_struct {
  __u32 id;
};

#endif /* defined(TEST_H) */
