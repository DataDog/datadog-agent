/* Defines 'redacted' as a string literal so that __DATE__/__TIME__ expansion
 * works in CPython source code (e.g. Modules/getbuildinfo.c).
 *
 * Hermetic toolchains redefine __DATE__, __TIME__, __TIMESTAMP__ to "redacted" for reproducible builds.
 * However, quotation can get lost when passing these definitions through flags on the shell.
 * Including this header (via CPPFLAGS)
 *
 * This is an alternative implementation of:
 * https://github.com/bazelbuild/rules_foreign_cc/issues/239#issuecomment-478167267
 * with two improvements:
 * - It doesn't get passed via configure_options, which would override the CPPFLAGS that
 *   rules_foreign_cc computes dynamically. Instead, this can be passed via setting
 *   "CPPFLAGS": "-include redacted_compat.h" in the `env`, which gets appended instead of overriding.
 * - Avoids having to figure out the correct escaping based on how commands may be invoked:
 *   this redefines the symbol `redacted` to `"redacted"` without ambiguity.
 */
#define redacted "redacted"
