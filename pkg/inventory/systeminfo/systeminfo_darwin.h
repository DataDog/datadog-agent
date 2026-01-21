#ifndef SYSTEMINFO_DARWIN_H
#define SYSTEMINFO_DARWIN_H

#include <stdbool.h>

typedef struct {
    char *modelNumber;
    char *serialNumber;
    char *productName;
    char *modelIdentifier;
} DeviceInfo;

DeviceInfo getDeviceInfo(void);

#endif
