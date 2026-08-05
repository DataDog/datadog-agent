#include <stdint.h>
#include <string.h>
#include <notify.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include "thermal_darwin.h"

// kIOMainPortDefault was introduced in macOS 12.0, use kIOMasterPortDefault
// for older versions.
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

// ---------------------------------------------------------------------
// AppleSMC struct-call plumbing. Raw binary struct calls into a private IOKit
// user client: no public header, no stability guarantee across macOS versions
// or hardware.
// ---------------------------------------------------------------------

#define KERNEL_INDEX_SMC      2

#define SMC_CMD_READ_BYTES    5
#define SMC_CMD_READ_KEYINFO  9

#define DATATYPE_SP78         "sp78"
#define DATATYPE_FLT          "flt "

typedef struct {
    UInt32 dataSize;
    UInt32 dataType;
    char   dataAttributes;
} SMCKeyData_keyInfo_t;

typedef char SMCBytes_t[32];

// pLimitData must stay 16 bytes (UInt16 version, UInt16 length, UInt32
// cpuPLimit, UInt32 gpuPLimit, UInt32 memPLimit) to match Apple's layout.
// Shrinking it to 14 silently corrupts every subsequent field.
typedef struct {
    UInt32               key;
    char                 vers[6];
    char                 pLimitData[16];
    SMCKeyData_keyInfo_t keyInfo;
    char                 result;
    char                 status;
    char                 data8;
    UInt32               data32;
    SMCBytes_t           bytes;
} SMCKeyData_t;

typedef char UInt32Char_t[5];

typedef struct {
    UInt32Char_t key;
    UInt32       dataSize;
    UInt32Char_t dataType;
    SMCBytes_t   bytes;
} SMCVal_t;

static io_connect_t smcConn;

static UInt32 smcStrToUL(const char *str, int size, int base) {
    UInt32 total = 0;
    int i;
    for (i = 0; i < size; i++) {
        if (base == 16) {
            total += (unsigned char)str[i] << (size - 1 - i) * 8;
        } else {
            total += (unsigned char)(str[i] << (size - 1 - i) * 8);
        }
    }
    return total;
}

static void smcULToStr(char *str, UInt32 val) {
    str[0] = '\0';
    sprintf(str, "%c%c%c%c",
            (unsigned int)val >> 24,
            (unsigned int)val >> 16,
            (unsigned int)val >> 8,
            (unsigned int)val);
}

static kern_return_t SMCOpen(void) {
    kern_return_t result;
    io_iterator_t iterator;
    io_object_t   device;

    CFMutableDictionaryRef matchingDictionary = IOServiceMatching("AppleSMC");
    result = IOServiceGetMatchingServices(IOKIT_MAIN_PORT, matchingDictionary, &iterator);
    if (result != kIOReturnSuccess) {
        return result;
    }

    device = IOIteratorNext(iterator);
    IOObjectRelease(iterator);
    if (device == 0) {
        return kIOReturnNotFound;
    }

    result = IOServiceOpen(device, mach_task_self(), 0, &smcConn);
    IOObjectRelease(device);
    return result;
}

static kern_return_t SMCClose(void) {
    return IOServiceClose(smcConn);
}

static kern_return_t SMCCall(int index, SMCKeyData_t *in, SMCKeyData_t *out) {
    size_t inSize = sizeof(SMCKeyData_t);
    size_t outSize = sizeof(SMCKeyData_t);
    return IOConnectCallStructMethod(smcConn, index, in, inSize, out, &outSize);
}

static kern_return_t SMCReadKey(const char *key, SMCVal_t *val) {
    kern_return_t result;
    SMCKeyData_t  inputStructure;
    SMCKeyData_t  outputStructure;

    memset(&inputStructure, 0, sizeof(SMCKeyData_t));
    memset(&outputStructure, 0, sizeof(SMCKeyData_t));
    memset(val, 0, sizeof(SMCVal_t));

    inputStructure.key = smcStrToUL(key, 4, 16);
    inputStructure.data8 = SMC_CMD_READ_KEYINFO;

    result = SMCCall(KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
    if (result != kIOReturnSuccess) {
        return result;
    }

    val->dataSize = outputStructure.keyInfo.dataSize;
    smcULToStr(val->dataType, outputStructure.keyInfo.dataType);
    inputStructure.keyInfo.dataSize = val->dataSize;
    inputStructure.data8 = SMC_CMD_READ_BYTES;

    result = SMCCall(KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
    if (result != kIOReturnSuccess) {
        return result;
    }

    memcpy(val->bytes, outputStructure.bytes, sizeof(outputStructure.bytes));
    return kIOReturnSuccess;
}

// smcFltToF reinterprets 4 raw bytes (little-endian, as SMC returns them) as
// an IEEE-754 float. Used for the "flt " data type.
static float smcFltToF(const unsigned char *b) {
    union { float f; unsigned char b[4]; } u;
    u.b[0] = b[0];
    u.b[1] = b[1];
    u.b[2] = b[2];
    u.b[3] = b[3];
    return u.f;
}

// SMCGetTemperature returns the key's value in °C, or 0.0 if the key does not
// exist, is not temperature-shaped, or the read fails. Missing keys are not
// reported as errors, so callers must range-filter the result.
static double SMCGetTemperature(const char *key) {
    SMCVal_t val;
    kern_return_t result = SMCReadKey(key, &val);
    if (result == kIOReturnSuccess && val.dataSize > 0) {
        if (strcmp(val.dataType, DATATYPE_SP78) == 0) {
            int intValue = ((unsigned char)val.bytes[0] * 256 + (unsigned char)val.bytes[1]) >> 2;
            return intValue / 64.0;
        } else if (strcmp(val.dataType, DATATYPE_FLT) == 0) {
            return (double)smcFltToF((unsigned char *)val.bytes);
        }
    }
    return 0.0;
}

// cpuSMCKeys are the per-chip-generation Apple Silicon CPU SMC keys.
static const char *cpuSMCKeys[] = {
    // M1 efficiency cores
    "Tp09", "Tp0T",
    // M1 performance cores
    "Tp01", "Tp05", "Tp0D", "Tp0H", "Tp0L", "Tp0P", "Tp0X", "Tp0b",
    // M1/M2 Pro/Max/Ultra
    "TC10", "TC11", "TC12", "TC13",
    "TC20", "TC21", "TC22", "TC23",
    "TC30", "TC31", "TC32", "TC33",
    "TC40", "TC41", "TC42", "TC43",
    "TC50", "TC51", "TC52", "TC53",
    // M2
    "Tp1h", "Tp1t", "Tp1p", "Tp1l",
    "Tp0j", "Tp0f",
    // M3
    "Te05", "Te0L", "Te0P", "Te0S",
    "Tf04", "Tf09", "Tf0A", "Tf0B", "Tf0D", "Tf0E",
    "Tf44", "Tf49", "Tf4A", "Tf4B", "Tf4D", "Tf4E",
    // M4
    "Te09", "Te0H",
    "Tp0V", "Tp0Y", "Tp0e",
};
static const int cpuSMCKeysCount = sizeof(cpuSMCKeys) / sizeof(cpuSMCKeys[0]);

// gpuSMCKeys are the per-chip-generation Apple Silicon GPU SMC keys.
static const char *gpuSMCKeys[] = {
    // M5
    "Tg0U", "Tg0X", "Tg0d", "Tg0g", "Tg0j", "Tg1Y", "Tg1c", "Tg1g",
    // M4 (base + Pro/Max/Ultra)
    "Tg0G", "Tg0H", "Tg1U", "Tg1k", "Tg0K", "Tg0L", "Tg0e", "Tg0k",
    // M3
    "Tf14", "Tf18", "Tf19", "Tf1A", "Tf24", "Tf28", "Tf29", "Tf2A",
    // M2
    "Tg0f", "Tg0j",
    // M1/M2 Pro/Max/Ultra
    "Tg04", "Tg0C", "Tg0K", "Tg0S",
    // M1
    "Tg05", "Tg0D", "Tg0L", "Tg0T",
};
static const int gpuSMCKeysCount = sizeof(gpuSMCKeys) / sizeof(gpuSMCKeys[0]);

// batterySMCKeys are the per-cell battery temperature SMC keys: TB0T/TB1T for
// a single/dual-cell pack, TB2T for a third cell on larger packs. A battery
// can plausibly read below 20°C, so these use a wider (0, 80) range filter
// than the (20, 150) used for CPU/GPU.
static const char *batterySMCKeys[] = {
    "TB0T", "TB1T", "TB2T",
};
static const int batterySMCKeysCount = sizeof(batterySMCKeys) / sizeof(batterySMCKeys[0]);

// ssdSMCKeys are the Intel/T2-era NVMe blade temperature SMC keys: TH0P plus
// up to 4 per-blade sensors TH0a-TH0d. Apple Silicon storage controllers also
// expose these. Not every key exists on a given machine; if none return a
// reading, the IOHID "NAND CH0 temp" node (HidInfo.nand) is the fallback.
static const char *ssdSMCKeys[] = {
    "TH0P", "TH0a", "TH0b", "TH0c", "TH0d",
};
static const int ssdSMCKeysCount = sizeof(ssdSMCKeys) / sizeof(ssdSMCKeys[0]);

// smcMaxOfKeysInRange returns the hottest reading in the exclusive
// (minC, maxC) range across keys/keyCount, taking the hottest core or die as
// the representative value. Returns {false, 0} if the connection cannot be
// opened or no key reads in range. The connection is opened once for the
// whole list.
static OptionalFloat smcMaxOfKeysInRange(const char **keys, int keyCount, double minC, double maxC) {
    OptionalFloat result = { false, 0.0f };

    if (SMCOpen() != kIOReturnSuccess) {
        return result;
    }

    for (int i = 0; i < keyCount; i++) {
        double t = SMCGetTemperature(keys[i]);
        if (t > minC && t < maxC) {
            if (!result.hasValue || t > result.value) {
                result.value = (float)t;
            }
            result.hasValue = true;
        }
    }

    SMCClose();
    return result;
}

// ---------------------------------------------------------------------
// IOHIDEventSystemClient plumbing. Part of the public IOKit.framework and
// needs no special privileges, but none of these functions ship public
// headers, so they are hand-declared here.
// ---------------------------------------------------------------------

typedef struct IOHIDServiceClient *IOHIDServiceClientRef;
typedef struct IOHIDEventSystemClient *IOHIDEventSystemClientRef;
typedef struct IOHIDEvent *IOHIDEventRef;

#define kHIDPage_AppleVendor 0xff00
#define kHIDUsage_AppleVendor_TemperatureSensor 0x0005
#define kIOHIDEventTypeTemperature 15

extern IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
extern int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef match);
extern CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
extern CFTypeRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef property);
extern IOHIDEventRef IOHIDServiceClientCopyEvent(IOHIDServiceClientRef service, long long type, int options, long long timestamp);
extern double IOHIDEventGetFloatValue(IOHIDEventRef event, long long field);

// HIDRawReading is one raw IOHID temperature sample: its "Product" name, an
// optional "LocationID" (used to de-duplicate physical nodes that are
// enumerated more than once), and its reading.
#define HID_MAX_READINGS 256

typedef struct {
    char    product[256];
    int64_t locationID;
    bool    hasLocationID;
    double  celsius;
} HIDRawReading;

// hidCollectRawReadings enumerates every IOHIDEventSystemClient service
// matching Apple's vendor-defined temperature sensor usage page/usage and
// fills out[] with every raw, plausible-range reading (no filtering by name,
// no de-duplication). Returns the number of entries written, or -1 if the
// IOHIDEventSystemClient/matching services could not be created.
static int hidCollectRawReadings(HIDRawReading *out, int outCapacity) {
    int page = kHIDPage_AppleVendor;
    int usage = kHIDUsage_AppleVendor_TemperatureSensor;
    CFNumberRef pageNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &page);
    CFNumberRef usageNum = CFNumberCreate(kCFAllocatorDefault, kCFNumberIntType, &usage);

    CFStringRef keys[2] = { CFSTR("PrimaryUsagePage"), CFSTR("PrimaryUsage") };
    CFNumberRef vals[2] = { pageNum, usageNum };

    CFDictionaryRef matching = CFDictionaryCreate(
        kCFAllocatorDefault,
        (const void **)keys,
        (const void **)vals,
        2,
        &kCFTypeDictionaryKeyCallBacks,
        &kCFTypeDictionaryValueCallBacks);
    CFRelease(pageNum);
    CFRelease(usageNum);

    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);
    if (system == NULL) {
        CFRelease(matching);
        return -1;
    }

    IOHIDEventSystemClientSetMatching(system, matching);
    CFArrayRef services = IOHIDEventSystemClientCopyServices(system);
    CFRelease(matching);
    if (services == NULL) {
        CFRelease(system);
        return -1;
    }

    CFIndex serviceCount = CFArrayGetCount(services);
    int n = 0;
    for (CFIndex i = 0; i < serviceCount && n < outCapacity; i++) {
        IOHIDServiceClientRef sc = (IOHIDServiceClientRef)CFArrayGetValueAtIndex(services, i);
        if (sc == NULL) {
            continue;
        }

        CFTypeRef nameRef = IOHIDServiceClientCopyProperty(sc, CFSTR("Product"));
        if (nameRef == NULL) {
            continue;
        }
        char product[256];
        product[0] = '\0';
        if (CFGetTypeID(nameRef) == CFStringGetTypeID()) {
            CFStringGetCString((CFStringRef)nameRef, product, sizeof(product), kCFStringEncodingUTF8);
        }
        CFRelease(nameRef);

        IOHIDEventRef event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypeTemperature, 0, 0);
        if (event == NULL) {
            continue;
        }
        double temp = IOHIDEventGetFloatValue(event, (long long)kIOHIDEventTypeTemperature << 16);
        CFRelease(event);
        if (temp <= 0.0 || temp > 150.0) {
            continue;
        }

        bool hasLocation = false;
        int64_t locationID = 0;
        CFTypeRef locRef = IOHIDServiceClientCopyProperty(sc, CFSTR("LocationID"));
        if (locRef != NULL) {
            if (CFGetTypeID(locRef) == CFNumberGetTypeID() &&
                CFNumberGetValue((CFNumberRef)locRef, kCFNumberSInt64Type, &locationID)) {
                hasLocation = true;
            }
            CFRelease(locRef);
        }

        strncpy(out[n].product, product, sizeof(out[n].product) - 1);
        out[n].product[sizeof(out[n].product) - 1] = '\0';
        out[n].locationID = locationID;
        out[n].hasLocationID = hasLocation;
        out[n].celsius = temp;
        n++;
    }

    CFRelease(services);
    CFRelease(system);
    return n;
}

// hidAverageMatching averages every reading in readings[0..count) whose
// product name satisfies matches(), first grouping repeated readings that
// share a LocationID into a single per-node average, so a node enumerated
// more than once isn't over-weighted relative to one that appears only once.
// Readings with no LocationID property are each treated as their own node.
// Returns {false, 0} if nothing matched.
#define HID_MAX_GROUPS 64

static OptionalFloat hidAverageMatching(HIDRawReading *readings, int count, bool (*matches)(const char *)) {
    int64_t groupLoc[HID_MAX_GROUPS];
    bool    groupHasLoc[HID_MAX_GROUPS];
    double  groupSum[HID_MAX_GROUPS];
    int     groupCount[HID_MAX_GROUPS];
    int     groupN = 0;

    for (int i = 0; i < count; i++) {
        HIDRawReading *r = &readings[i];
        if (!matches(r->product)) {
            continue;
        }

        int idx = -1;
        if (r->hasLocationID) {
            for (int g = 0; g < groupN; g++) {
                if (groupHasLoc[g] && groupLoc[g] == r->locationID) {
                    idx = g;
                    break;
                }
            }
        }
        if (idx < 0) {
            if (groupN >= HID_MAX_GROUPS) {
                continue;
            }
            idx = groupN++;
            groupHasLoc[idx] = r->hasLocationID;
            groupLoc[idx] = r->locationID;
            groupSum[idx] = 0.0;
            groupCount[idx] = 0;
        }
        groupSum[idx] += r->celsius;
        groupCount[idx]++;
    }

    OptionalFloat result = { false, 0.0f };
    if (groupN == 0) {
        return result;
    }

    double total = 0.0;
    for (int g = 0; g < groupN; g++) {
        total += groupSum[g] / groupCount[g];
    }
    result.hasValue = true;
    result.value = (float)(total / groupN);
    return result;
}

// hidMaxMatching groups readings[0..count) whose product name satisfies
// matches() exactly like hidAverageMatching (per-LocationID average, each
// node counted once), but returns the hottest group average instead of the
// mean across groups — the single hottest node contributing to that sensor.
// Returns {false, 0} if nothing matched.
static OptionalFloat hidMaxMatching(HIDRawReading *readings, int count, bool (*matches)(const char *)) {
    int64_t groupLoc[HID_MAX_GROUPS];
    bool    groupHasLoc[HID_MAX_GROUPS];
    double  groupSum[HID_MAX_GROUPS];
    int     groupCount[HID_MAX_GROUPS];
    int     groupN = 0;

    for (int i = 0; i < count; i++) {
        HIDRawReading *r = &readings[i];
        if (!matches(r->product)) {
            continue;
        }

        int idx = -1;
        if (r->hasLocationID) {
            for (int g = 0; g < groupN; g++) {
                if (groupHasLoc[g] && groupLoc[g] == r->locationID) {
                    idx = g;
                    break;
                }
            }
        }
        if (idx < 0) {
            if (groupN >= HID_MAX_GROUPS) {
                continue;
            }
            idx = groupN++;
            groupHasLoc[idx] = r->hasLocationID;
            groupLoc[idx] = r->locationID;
            groupSum[idx] = 0.0;
            groupCount[idx] = 0;
        }
        groupSum[idx] += r->celsius;
        groupCount[idx]++;
    }

    OptionalFloat result = { false, 0.0f };
    for (int g = 0; g < groupN; g++) {
        double avg = groupSum[g] / groupCount[g];
        if (!result.hasValue || avg > result.value) {
            result.hasValue = true;
            result.value = (float)avg;
        }
    }
    return result;
}

// Sensor name matchers. Naming differs by Apple Silicon generation and is
// not universal. "PMU tdie*"
// (as opposed to "PMU2 tdie*", the GPU/media-engine die on two-die
// "Fusion" packages) is used as the SoC/CPU-die approximation.
static bool matchesTdie(const char *product) {
    return strncmp(product, "PMU tdie", 8) == 0;
}

static bool matchesNand(const char *product) {
    return strcmp(product, "NAND CH0 temp") == 0;
}

static bool matchesBattery(const char *product) {
    return strcmp(product, "gas gauge battery") == 0;
}

static bool matchesPACC(const char *product) {
    return strncmp(product, "pACC MTR Temp Sensor", 21) == 0;
}

static bool matchesEACC(const char *product) {
    return strncmp(product, "eACC MTR Temp Sensor", 21) == 0;
}

static bool matchesGPUMTR(const char *product) {
    return strncmp(product, "GPU MTR Temp Sensor", 20) == 0;
}

// ---------------------------------------------------------------------
// Thermal pressure level, via the Darwin notification API.
// ---------------------------------------------------------------------

// getThermalPressureLevel returns the raw thermal pressure level (0=Nominal,
// 1=Moderate, 2=Heavy, 3=Trapping, 4=Sleeping) from the private
// "com.apple.system.thermalpressurelevel" notification, or {false, 0} if
// registration or the state lookup failed.
static OptionalInt getThermalPressureLevel(void) {
    OptionalInt result = { false, 0 };

    int token;
    if (notify_register_check("com.apple.system.thermalpressurelevel", &token) != NOTIFY_STATUS_OK) {
        return result;
    }

    uint64_t state;
    uint32_t status = notify_get_state(token, &state);
    notify_cancel(token);

    if (status != NOTIFY_STATUS_OK) {
        return result;
    }
    result.hasValue = true;
    result.value = (int)state;
    return result;
}

ThermalInfo getThermalInfo(void) {
    ThermalInfo info;
    memset(&info, 0, sizeof(info));

    info.smc.cpu = smcMaxOfKeysInRange(cpuSMCKeys, cpuSMCKeysCount, 20.0, 150.0);
    info.smc.gpu = smcMaxOfKeysInRange(gpuSMCKeys, gpuSMCKeysCount, 20.0, 150.0);
    info.smc.ssd = smcMaxOfKeysInRange(ssdSMCKeys, ssdSMCKeysCount, 20.0, 150.0);
    info.smc.battery = smcMaxOfKeysInRange(batterySMCKeys, batterySMCKeysCount, 0.0, 80.0);

    HIDRawReading readings[HID_MAX_READINGS];
    int n = hidCollectRawReadings(readings, HID_MAX_READINGS);
    if (n > 0) {
        info.hid.tdie = hidAverageMatching(readings, n, matchesTdie);
        info.hid.nand = hidAverageMatching(readings, n, matchesNand);
        info.hid.battery = hidAverageMatching(readings, n, matchesBattery);
        info.hid.mtrPacc = hidAverageMatching(readings, n, matchesPACC);
        info.hid.mtrEacc = hidAverageMatching(readings, n, matchesEACC);
        info.hid.mtrGpu = hidAverageMatching(readings, n, matchesGPUMTR);

        info.hid.tdieMax = hidMaxMatching(readings, n, matchesTdie);
        info.hid.nandMax = hidMaxMatching(readings, n, matchesNand);
        info.hid.batteryMax = hidMaxMatching(readings, n, matchesBattery);
        info.hid.mtrPaccMax = hidMaxMatching(readings, n, matchesPACC);
        info.hid.mtrEaccMax = hidMaxMatching(readings, n, matchesEACC);
        info.hid.mtrGpuMax = hidMaxMatching(readings, n, matchesGPUMTR);
    }

    info.thermalLevel = getThermalPressureLevel();

    return info;
}
