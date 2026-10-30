#ifndef SYSTEMINFO_DARWIN_H
#define SYSTEMINFO_DARWIN_H

#include <stdbool.h>

typedef struct {
    char *modelNumber;
    char *serialNumber;
    char *productName;
    char *modelIdentifier;
    char *computerName;
} DeviceInfo;

DeviceInfo getDeviceInfo(void);
// Returns an owned UTF-8 string, or NULL when unavailable. The caller must free it.
char *copyLocalHostName(void);

#endif
