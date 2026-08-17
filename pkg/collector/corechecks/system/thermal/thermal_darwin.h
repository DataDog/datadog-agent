#ifndef THERMAL_DARWIN_H
#define THERMAL_DARWIN_H

#include <stdbool.h>

typedef struct {
    bool  hasValue;
    float value;
} OptionalFloat;

typedef struct {
    bool hasValue;
    int  value;
} OptionalInt;

// SmcInfo holds the thermal sensors read via the AppleSMC user client. Each
// field has no value if no key for that sensor read in range.
typedef struct {
    OptionalFloat cpu;
    OptionalFloat gpu;
    OptionalFloat ssd;
    OptionalFloat battery;
} SmcInfo;

typedef struct {
    SmcInfo smc;
    // thermalLevel is the raw macOS thermal pressure level (0=Nominal,
    // 1=Moderate, 2=Heavy, 3=Trapping, 4=Sleeping) from the private
    // "com.apple.system.thermalpressurelevel" notification, with no value if
    // the lookup failed.
    OptionalInt thermalLevel;
} ThermalInfo;

ThermalInfo getThermalInfo(void);

#endif
