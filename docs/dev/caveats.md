# Development Caveats

This document provides a list of known development caveats

## Windows

The COM concurrency model may be set in different ways, it also has to be called for every thread that might indeed interact with the COM library. Furthermore, once a concurrency model is set for a thread, it cannot be changed unless the thread is `CoUnitilialize()`d. This poses an issue for us for a variety of reasons:
1. We use thirdparty libraries like `gopsutil` that initialize the concurrency model setting it to the multi-threaded model - the libary will fail in its calls if the model is any different.
2. We also have python integrations that employ the COM library (ie. WMI, SQLserver, ...) that ultimately rely on `pythoncom` for this. `pythoncom`, in fact, initializes the COM library to the single-threaded model by default, but doesn't really care about the concurrency model and will not fail if a different model has been previously set. 
3. Because the actual *loading* of the integrations will import `pythoncom` the concurrency model might be inadvertedly and implicitly be set to the default (single-threaded) concurrency model meaning that any subsequent call to an affected `gopsutil` function would fail as the concurrency model would already be set. 
4. Due to go's concurrency model we can assume nothing about what goroutine is running on what thread at any given time, so it's not trivial to tell what concurrency model a thread's COM library was initialized to. 
 
Since we only need to invoke `gopsutil` functions that rely on COM calls (requiring the multi-threaded concurrency model) during agent initialization, we can make sure that all involved threads are set to the multi-threaded model _before_ checks are run. We achieve this in the python loader by calling `CoInitializeEx(0)` while checks are getting loaded, and running `CoUninitialize()` immediately after loading. By doing so, when `pythoncom` is imported during the loading of checks the concurrency model is already set
and involved go checks and facilities (CPU check, which calls `gopsutil` to collect CPU information during its configure phase) may be set up successfully. 

Once the agent is finally up, and we get past the check setup, no additional COM calls will currently be made from go-land. However, we do continue to make these calls from python checks. Python checks will set/use the concurrency model as they please (typically the single-threaded model). This has a few implications:
- We cannot assume anything about the concurrency model of a thread after checks are loaded. 
- Any call to a `gopsutil` that might rely on WMI is liable to failure as it might try to initialize on a multi-threaded model while the thread might be on the single-threaded one.  
- Since we call `CoInitializeEx(0)` when loading checks, the current behavior would break Auto-Discovery and dynamic reload of checks on windows.



