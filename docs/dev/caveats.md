# Development Caveats

This document provides a list of known development caveats

## Windows

### COM Caveats

The COM concurrency model may be set in different ways, it also has to be called for every thread that might indeed interact with the COM library. Furthermore, once a concurrency model is set for a thread, it cannot be changed unless the thread is `CoUnitilialize()`d. This poses an issue for us for a variety of reasons:
1. Third party libraries like `gopsutil`, which initializes the concurrency model by setting it to the multi-threaded model, will fail in its calls as the model is different.
2. We also have python integrations that employ the COM library (ie. WMI, SQLserver, ...) that ultimately rely on `pythoncom` for this. `pythoncom`, in fact, initializes the COM library to the single-threaded model by default, but doesn't really care about the concurrency model and will not fail if a different model has been previously set. 
3. Because the actual *loading* of the integrations will import `pythoncom` the concurrency model might be inadvertently and implicitly be set to the default (single-threaded) concurrency model meaning that any subsequent call to an affected `gopsutil` function would fail as the concurrency model would already be set. 
4. Due to go's concurrency model we can assume nothing about what goroutine is running on what thread at any given time, so it's not trivial to tell what concurrency model a thread's COM library was initialized to. 


To support Python checks which use COM, we call  `CoInitializeEx(0)` in the python loader while checks are loading, and run `CoUninitialize()` immediately after loading. By doing so, `pythoncom` is imported during the loading of checks and the concurrency model is already set.
### WMI
WMI is implemented via COM. Therefore, all of the above apply to any code that directly or indirectly uses WMI.  _All_ use of WMI in the core agent is removed, along with the [removal](https://github.com/DataDog/datadog-agent/blob/main/tasks/go.py#L295-L299) of the `gopsutil` dependency to ensure that this dependency is not reintroduced.


However, Python checks do continue to make these calls. Python checks will set/use the concurrency model as they please (typically the single-threaded model). This has a few implications:
- We cannot assume anything about the concurrency model of a thread after checks are loaded. 
- Any call to a library that might rely on WMI is liable to fail as it might try to initialize on a multi-threaded model while the thread might be on the single-threaded one.  
- Since we call `CoInitializeEx(0)` when loading checks, the current behavior would break Auto-Discovery and dynamic reload of checks on windows.

WMI in general is not very stable. A great deal of effort has been made to remove WMI from the core agent.  Additional effort has been made to remove WMI from the Python checks. Take care to not (re)introduce WMI into the Agent.
