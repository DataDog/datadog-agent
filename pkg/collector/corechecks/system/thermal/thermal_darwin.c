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

// The SMC connection is passed as a parameter rather than held in a global:
// thermal instances run concurrently on separate collector workers, and a
// shared handle would let one call close or overwrite a connection another is
// still reading through, leaking the overwritten mach port.

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

// SMCOpen stores an open AppleSMC connection in *conn. On failure *conn is
// left untouched and must not be used.
static kern_return_t SMCOpen(io_connect_t *conn) {
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

    result = IOServiceOpen(device, mach_task_self(), 0, conn);
    IOObjectRelease(device);
    return result;
}

static kern_return_t SMCClose(io_connect_t conn) {
    return IOServiceClose(conn);
}

static kern_return_t SMCCall(io_connect_t conn, int index, SMCKeyData_t *in, SMCKeyData_t *out) {
    size_t inSize = sizeof(SMCKeyData_t);
    size_t outSize = sizeof(SMCKeyData_t);
    return IOConnectCallStructMethod(conn, index, in, inSize, out, &outSize);
}

static kern_return_t SMCReadKey(io_connect_t conn, const char *key, SMCVal_t *val) {
    kern_return_t result;
    SMCKeyData_t  inputStructure;
    SMCKeyData_t  outputStructure;

    memset(&inputStructure, 0, sizeof(SMCKeyData_t));
    memset(&outputStructure, 0, sizeof(SMCKeyData_t));
    memset(val, 0, sizeof(SMCVal_t));

    inputStructure.key = smcStrToUL(key, 4, 16);
    inputStructure.data8 = SMC_CMD_READ_KEYINFO;

    result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
    if (result != kIOReturnSuccess) {
        return result;
    }

    val->dataSize = outputStructure.keyInfo.dataSize;
    smcULToStr(val->dataType, outputStructure.keyInfo.dataType);
    inputStructure.keyInfo.dataSize = val->dataSize;
    inputStructure.data8 = SMC_CMD_READ_BYTES;

    result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
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
static double SMCGetTemperature(io_connect_t conn, const char *key) {
    SMCVal_t val;
    kern_return_t result = SMCReadKey(conn, key, &val);
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

// cpuSMCKeys are the CPU SMC keys for every supported Mac. Apple Silicon and
// Intel keys are probed unconditionally: a key absent on the running machine
// reads as 0.0 and is dropped by the range filter in smcMaxOfKeysInRange, so
// each family is inert on the other architecture.
//
// SMC keys are case-sensitive: Tg0D and TG0D are different sensors.
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
    // Intel: package/die/proximity. Distinct from the Apple Silicon
    // "Tp"/"Te"/"Tf" keys and from the TC10-TC53 cluster keys above.
    "TC0D", "TC0E", "TC0F", "TC0H", "TC0P",
    "TCAD", "TCXC",
    // Intel: per-core die temperatures.
    "TC0C", "TC1C", "TC2C", "TC3C", "TC4C", "TC5C", "TC6C", "TC7C",
};
static const int cpuSMCKeysCount = sizeof(cpuSMCKeys) / sizeof(cpuSMCKeys[0]);

// gpuSMCKeys are the GPU SMC keys for every supported Mac, probed
// unconditionally for the same reason as cpuSMCKeys.
//
// A key reused across chip generations is listed once, under the first
// generation that uses it, and the later group notes the reuse.
static const char *gpuSMCKeys[] = {
    // M5
    "Tg0U", "Tg0X", "Tg0d", "Tg0g", "Tg0j", "Tg1Y", "Tg1c", "Tg1g",
    // M4 (base + Pro/Max/Ultra)
    "Tg0G", "Tg0H", "Tg1U", "Tg1k", "Tg0K", "Tg0L", "Tg0e", "Tg0k",
    // M3
    "Tf14", "Tf18", "Tf19", "Tf1A", "Tf24", "Tf28", "Tf29", "Tf2A",
    // M2 (also uses Tg0j, listed under M5)
    "Tg0f",
    // M1/M2 Pro/Max/Ultra (also uses Tg0K, listed under M4)
    "Tg04", "Tg0C", "Tg0S",
    // M1 (also uses Tg0L, listed under M4)
    "Tg05", "Tg0D", "Tg0T",
    // Intel discrete GPU: die/proximity/heatsink for up to two GPUs. Uppercase
    // "TG", distinct from the lowercase "Tg" Apple Silicon keys above.
    "TG0D", "TG0H", "TG0P", "TG1D", "TG1H", "TG1P",
    // Intel integrated graphics, reported on the CPU package. Covers Intel
    // Macs with no discrete GPU, where the TG* keys are absent.
    "TCGC",
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
// expose these. Not every key exists on a given machine.
static const char *ssdSMCKeys[] = {
    "TH0P", "TH0a", "TH0b", "TH0c", "TH0d",
};
static const int ssdSMCKeysCount = sizeof(ssdSMCKeys) / sizeof(ssdSMCKeys[0]);

// smcMaxOfKeysInRange returns the hottest reading in the exclusive
// (minC, maxC) range across keys/keyCount, taking the hottest core or die as
// the representative value. Returns {false, 0} if the connection cannot be
// opened or no key reads in range.
//
// The connection is opened once for the whole list and is local to this call,
// so concurrent invocations cannot disturb each other.
static OptionalFloat smcMaxOfKeysInRange(const char **keys, int keyCount, double minC, double maxC) {
    OptionalFloat result = { false, 0.0f };
    io_connect_t  conn = MACH_PORT_NULL;

    if (SMCOpen(&conn) != kIOReturnSuccess) {
        return result;
    }

    for (int i = 0; i < keyCount; i++) {
        double t = SMCGetTemperature(conn, keys[i]);
        if (t > minC && t < maxC) {
            if (!result.hasValue || t > result.value) {
                result.value = (float)t;
            }
            result.hasValue = true;
        }
    }

    SMCClose(conn);
    return result;
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

    info.thermalLevel = getThermalPressureLevel();

    return info;
}
