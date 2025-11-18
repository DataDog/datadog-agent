#include "liba.h"

#include <math.h>

#ifndef REQUIRED_DEFINE
// '-DREQUIRED_DEFINE' is set via the copts attribute of the make rule.
#error "REQUIRED_DEFINE is not defined"
#endif

std::string hello_liba(void) { return "Hello from LIBA!"; }

double hello_math(double a) {
    // On Unix, this call requires linking to libm.so. The Bazel toolchain adds
    // the required `-lm` linkopt automatically and rules_foreign_cc forwards
    // this option to make invocation via the CXXFLAGS variable.
    return acos(a);
}
