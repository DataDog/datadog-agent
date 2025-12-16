#ifndef HOSTHARDWARE_DARWIN_H
#define HOSTHARDWARE_DARWIN_H

#include <stdbool.h>

typedef struct {
    char *modelIdentifier;
    char *modelNumber;
    char *productName;
    char *serialNumber;
} DeviceInfo;

DeviceInfo getDeviceInfo(void);

#endif
