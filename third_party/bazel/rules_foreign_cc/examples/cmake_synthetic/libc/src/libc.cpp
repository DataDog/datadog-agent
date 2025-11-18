#include "libc.h"

std::string hello_libc(void) {
    return hello_liba() + " " + hello_libb() + " Hello from LIBC!";
}
