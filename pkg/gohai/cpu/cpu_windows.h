#ifndef CPU_WINDOWS_H
#define CPU_WINDOWS_H

// CPU_INFO mirrors the Go struct
typedef struct {
    int corecount;
    int logicalcount;
    int pkgcount;
    int numaNodeCount;
    int relationGroups;
    int maxProcsInGroups;
    int activeProcsInGroups;
    unsigned long long l1CacheSize;
    unsigned long long l2CacheSize;
    unsigned long long l3CacheSize;
} CPU_INFO;

// computeCoresAndProcessors gets CPU information using Windows API
int computeCoresAndProcessors(CPU_INFO* cpuInfo);

#endif // CPU_WINDOWS_H 