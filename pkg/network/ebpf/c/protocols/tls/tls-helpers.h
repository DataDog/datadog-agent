#ifndef __TLS_HELPERS_H
#define __TLS_HELPERS_H

#include "tracer/tracer.h"

// Assume that a zero value for chosen_version and cipher_suite indicates "not set"
#define TLS_VERSION_UNSET 0
#define CIPHER_SUITE_UNSET 0

// merge_tls_info modifies `this` by merging it with `that`
static __always_inline void merge_tls_info(tls_info_t *this, tls_info_t *that) {
    if (!this || !that) {
        return;
    }

    // Merge chosen_version if not already set
    if (this->chosen_version == TLS_VERSION_UNSET && that->chosen_version != TLS_VERSION_UNSET) {
        this->chosen_version = that->chosen_version;
    }

    // Merge cipher_suite if not already set
    if (this->cipher_suite == CIPHER_SUITE_UNSET && that->cipher_suite != CIPHER_SUITE_UNSET) {
        this->cipher_suite = that->cipher_suite;
    }

    // Merge offered_versions bitmask using bitwise OR
    this->offered_versions |= that->offered_versions;

    // Merge reserved field if necessary (depending on your use case)
    // For now, we can choose to keep it as is or apply specific logic
    // this->reserved |= that->reserved; // Uncomment if needed
}

#endif