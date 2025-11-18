#ifndef FOO
#error FOO is not defined
#endif

#define XSTR(x) STR(x)
#define STR(x) #x
#pragma message "The value of __TIME__: " XSTR(__TIME__)

#define STATIC_ASSERT(condition, name) \
    typedef char assert_failed_##name[(condition) ? 1 : -1];

void foo() { STATIC_ASSERT(__TIME__ == "redacted", time_must_be_redacted); }

// Should return "redacted"
const char *getBuildTime(void) { return __TIME__; }
