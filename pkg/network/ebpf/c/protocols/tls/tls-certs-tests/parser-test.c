#include <stdlib.h>
#include <stdio.h>
#include <errno.h>


#define TEST_BUILD_NO_EBPF

#include "../tls-certs-parser.h"


#define bail(format, ...) { printf(format "\n", ##__VA_ARGS__); exit(1); }


long read_file(char *path, char** buffer) {
    FILE *fp = fopen(path, "rb");
    if (!fp) {
        bail("failed to fopen '%s': %d", path, errno);
    }

    int err = fseek(fp, 0, SEEK_END);
    if (err) {
        bail("fseek SEEK_END error: %d", err);
    }

    long size = ftell(fp);
    if (size < 0) {
        bail("ftell error: %d", errno);
    }

    err = fseek(fp, 0, SEEK_SET);
    if (err) {
        bail("fseek SEEK_SET error: %d", err);
    }

    *buffer = calloc(size, 1);
    if (!*buffer) {
        bail("malloc failed");
    }

    int written = fread(*buffer, size, 1, fp);
    if (written != 1) {
        bail("fread failed for size %ld: %d", size, written);
    }

    fclose(fp);

    return size;
}

void hexdump(char *data, size_t size) {
    for (size_t i=0; i<size; i++) {
        if (i > 0 && i % 20 == 0) {
            printf("\n");
        }
        printf("%02x ", data[i]);
    }
    printf("\n");
}

bool memcmp_len(char *a_buf, size_t a_size, char *b_buf, size_t b_size) {
    if (a_size != b_size) {
        return false;
    }

    int cmp = memcmp(a_buf, b_buf, a_size);
    return cmp == 0;
}
bool matches_utc(char *test_name, char *kind, char *expected, char *actual) {
    bool matches = !memcmp(expected, actual, UTC_ZONELESS_LEN);
    if (!matches) {
        printf("[%s] mismatched %s, expected:\n", test_name, kind);
        printf("    %.*s\n", UTC_ZONELESS_LEN, expected);
        printf("actual:\n");
        printf("    %.*s\n", UTC_ZONELESS_LEN, actual);
    }

    return matches;
}

bool check_memcmp_len(char *test_name, cert_t expected, cert_t actual) {
    bool passed = true;

    if (expected.is_ca != actual.is_ca) {
        passed = false;

        printf("[%s] mismatched is_ca.\n", test_name);
        printf("expected: %d\n", expected.is_ca);
        printf("  actual: %d\n", actual.is_ca);
    }

    if (!memcmp_len(expected.serial.data, expected.serial.len, actual.serial.data, actual.serial.len)) {
        passed = false;

        printf("[%s] mismatched serial.\n", test_name);
        printf("expected: ");
        hexdump(expected.serial.data, expected.serial.len);
        printf("  actual: ");
        hexdump(actual.serial.data, actual.serial.len);
    }

    if (!memcmp_len(expected.domain.data, expected.domain.len, actual.domain.data, actual.domain.len)) {
        passed = false;

        printf("[%s] mismatched domain.\n", test_name);
        printf("expected: '%.*s'\n", expected.domain.len, expected.domain.data);
        printf("  actual: '%.*s'\n", actual.domain.len, actual.domain.data);
    }

    if (!matches_utc(test_name, "not_before", expected.validity.not_before, actual.validity.not_before)) {
        passed = false;
    }
    if (!matches_utc(test_name, "not_after", expected.validity.not_after, actual.validity.not_after)) {
        passed = false;
    }

    if (!passed) {
        printf("========\n");
    }

    return passed;
}

bool test_datadoghq() {
    char *buffer;
    long size = read_file("datadoghq.der", &buffer);

    data_t data = { buffer, buffer + size };
    cert_t actual = {0};
    bool failed = parse_cert(data, &actual);
    if (failed) {
        printf("datadoghq parse_cert failed\n");
        return false;
    }
    free(buffer);

    cert_t dd_cert = {0};
    char expected_serial[] = {0x07, 0x7C, 0x68, 0xDF, 0xBA, 0x21, 0x15, 0x28, 0xFA, 0xB6, 0x4E, 0x47, 0xC5, 0x1C, 0x7E, 0xB7};
    dd_cert.serial.len = sizeof(expected_serial);
    memcpy(dd_cert.serial.data, expected_serial, sizeof(expected_serial));
    strncpy(dd_cert.validity.not_before, "250702000000", UTC_ZONELESS_LEN);
    strncpy(dd_cert.validity.not_after, "260702235959", UTC_ZONELESS_LEN);

    const char *domain = "*.datadoghq.com";
    dd_cert.domain.len = strlen(domain);
    strcpy(dd_cert.domain.data, domain);


    return check_memcmp_len("datadoghq", dd_cert, actual);
}


bool test_digicert_ca() {
    char *buffer;
    long size = read_file("digicert_ca.der", &buffer);

    data_t data = { buffer, buffer + size };
    cert_t actual = {0};
    bool failed = parse_cert(data, &actual);
    if (failed) {
        printf("datadoghq parse_cert failed\n");
        return false;
    }
    free(buffer);

    cert_t dd_cert = {0};
    dd_cert.is_ca = true;
    char expected_serial[] = {0x0C, 0xF5, 0xBD, 0x06, 0x2B, 0x56, 0x02, 0xF4, 0x7A, 0xB8, 0x50, 0x2C, 0x23, 0xCC, 0xF0, 0x66};
    dd_cert.serial.len = sizeof(expected_serial);
    memcpy(dd_cert.serial.data, expected_serial, sizeof(expected_serial));
    strncpy(dd_cert.validity.not_before, "210330000000", UTC_ZONELESS_LEN);
    strncpy(dd_cert.validity.not_after, "310329235959", UTC_ZONELESS_LEN);

    return check_memcmp_len("digicert_ca", dd_cert, actual);
}

int main(int argc, char **argv) {
    int fails = 0;
    if (!test_datadoghq()) {
        fails++;
    }
    if (!test_digicert_ca()) {
        fails++;
    }

    if (fails > 0) {
        printf("%d tests failed\n", fails);
        return 1;
    }

    printf("all tests passed\n");

    return 0;
}
