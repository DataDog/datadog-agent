/* Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2025-present Datadog, Inc. */

/*
 * C test for the FFI panic-catch path in dd_discovery_get_services.
 *
 * This test links against a build of the library compiled with the
 * `force-ffi-panic` Cargo feature, which causes dd_discovery_get_services to
 * unconditionally panic. The test verifies that the panic is caught and NULL
 * is returned rather than the process aborting or exhibiting undefined
 * behaviour from an unwind across the C ABI boundary.
 */

#include <stdio.h>

#include "dd_discovery.h"

int main(void) {
    struct dd_discovery_result *result =
        dd_discovery_get_services(NULL, 0, NULL, 0);

    if (result != NULL) {
        fprintf(stderr,
                "FAIL: expected NULL from dd_discovery_get_services on panic, "
                "got non-NULL pointer\n");
        /* Free to avoid leaking memory in case the test is ever run without
         * the force-ffi-panic feature. */
        dd_discovery_free(result);
        return 1;
    }

    printf("PASS: dd_discovery_get_services returned NULL on panic\n");
    return 0;
}
