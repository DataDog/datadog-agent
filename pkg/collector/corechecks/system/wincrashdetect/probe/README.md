## How this works, and how to test.

### How this works

Windows provides a limited subset of the debugger functionality with the base OS.  This allows us to execute some (not all) debugger commands when we discover a crash dump file.  But, the output is very
limited.  It simply dumps the strings that would appear in the debugger.  So we have to parse the string
to find the parts we're interested.  A sample output is supplied below.

### How to (really) test

Unfortunately, actually testing the parser then requires actual dumps.  Actual dumps require multiple gigabytes of data.  To manually test, in a VM (don't do this on your own machine), install the crasher driver (TBD: linked here).  Start the driver. It will _immediately_ crash.  On reboot there will be a crash dump to parse, and (assuming the agent is installed) the agent will find and report.

## Sample output from debugger callback.

Note that the string below is somewhat formatted.  In practice, the string is sent to the callback
in chunks, not necessarily, for example, on newline boundaries.

=== cut here ===

Kernel Bitmap Dump File: Kernel address space is available, User address space may not be available.

Symbol search path is: srv*
Executable search path is:
*** ERROR: Symbol file could not be found.  Defaulted to export symbols for ntkrnlmp.exe -
Windows 10 Kernel Version 14393 MP (2 procs) Free x64
Product: Server, suite: TerminalServer SingleUserTS
Built by: 14393.5989.amd64fre.GitEnlistment.230602-1907
Machine Name:
Kernel base = 0xfffff800`4567f000 PsLoadedModuleList = 0xfffff800`45984cf0
Debug session time: Mon Jun 26 20:44:49.742 2023 (UTC - 7:00)
System Uptime: 0 days 0:03:58.449
*** ERROR: Symbol file could not be found.  Defaulted to export symbols for ntkrnlmp.exe -
Loading Kernel Symbols
...............................................................
................................................................
........................
Loading User Symbols

Loading unloaded module list
...........

************* Symbol Loading Error Summary **************
Module name            Error
ntkrnlmp               The system cannot find the file specified

You can troubleshoot most symbol related issues by turning on symbol loading diagnostics (!sym noisy) and repeating the
command that caused symbols to be loaded.
You should also verify that your symbol search path (.sympath) is correct.
Unable to add extension DLL: kdexts
Unable to add extension DLL: kext
Unable to add extension DLL: exts
The call to LoadLibrary(ext) failed, Win32 error 0n2
    "The system cannot find the file specified."
Please check your debugger configuration and/or network access.
The call to LoadLibrary(ext) failed, Win32 error 0n2
    "The system cannot find the file specified."
Please check your debugger configuration and/or network access.
*******************************************************************************
*                                                                             *
*                        Bugcheck Analysis                                    *
*                                                                             *
*******************************************************************************
Bugcheck code 0000007E
Arguments ffffffff`c0000005 fffff806`f7e010e6 ffffb481`789326a8 ffffb481`78931ef0

RetAddr           : Args to Child                                                           : Call Site
fffff800`457f4db0 : 00000000`0000007e ffffffff`c0000005 fffff806`f7e010e6 ffffb481`789326a8 : nt!KeBugCheckEx
fffff800`457cb7bf : 00000000`00000004 00000000`00000000 00007fff`ffff0000 ffffc582`1b4e3800 : nt!memset+0x5530
fffff800`457e602d : ffffb481`78933000 ffffb481`789318c0 00000000`00000000 00000000`00000050 : nt!_C_specific_handler+0x9
f
fffff800`457742a1 : ffffb481`78933000 00000000`00000000 ffffb481`7892d000 00000000`00000000 : nt!_chkstk+0x5d
fffff800`457730c4 : ffffb481`789326a8 ffffb481`789323f0 ffffb481`789326a8 ffffb481`78932570 : nt!KeQuerySystemTimePrecis
e+0x27d1
fffff800`457ee482 : 00003c74`00000000 fffff800`458a1d00 00000000`00000000 fffff800`45d940c4 : nt!KeQuerySystemTimePrecis
e+0x15f4
fffff800`457eafc0 : 00000000`00000000 fffff800`45a97fe0 ffff8301`2c077220 ffffc582`1bb72c30 : nt!setjmpex+0x7622
fffff806`f7e010e6 : 00000000`00000001 00000000`00000000 ffffb481`76e2e000 fffff800`456e6511 : nt!setjmpex+0x4160
*** ERROR: Module load completed but symbols could not be loaded for ddapmcrash.sys
fffff806`f7e07020 : ffffc582`1bb72c30 ffffc582`19f18000 ffffc582`1bb72c30 ffff3ac8`f399d666 : ddapmcrash+0x10e6
fffff800`45b338f7 : 00000000`00000000 00000000`00000000 ffffc582`1bb72c30 ffffffff`000001c8 : ddapmcrash+0x7020
fffff800`45ad140e : 00000000`00000000 00000000`00000000 00000000`00000000 fffff800`45a3e2c0 : nt!FsRtlNotifyVolumeEventE
x+0x243b
fffff800`45715dc9 : fffff800`00000000 ffffffff`80000ba4 ffffc582`1b4e3800 fffff800`45a3e2c0 : nt!MmGetPhysicalMemoryRang
esEx+0xb56
fffff800`456c6f85 : ffffc582`1b4e3800 00000000`00000080 ffffc582`18a636c0 ffffc582`1b4e3800 : nt!KdPollBreakIn+0x8059
fffff800`457e4df6 : ffffb481`76e15180 ffffc582`1b4e3800 fffff800`456c6f44 00000000`00000246 : nt!PsGetProcessSessionIdEx
+0x2d5
00000000`00000000 : ffffb481`78933000 ffffb481`7892d000 00000000`00000000 00000000`00000000 : nt!KeSynchronizeExecution+
0x7756

RetAddr           : Args to Child                                                           : Call Site
fffff800`457f4db0 : 00000000`0000007e ffffffff`c0000005 fffff806`f7e010e6 ffffb481`789326a8 : nt!KeBugCheckEx
fffff800`457cb7bf : 00000000`00000004 00000000`00000000 00007fff`ffff0000 ffffc582`1b4e3800 : nt!memset+0x5530
fffff800`457e602d : ffffb481`78933000 ffffb481`789318c0 00000000`00000000 00000000`00000050 : nt!_C_specific_handler+0x9
f
fffff800`457742a1 : ffffb481`78933000 00000000`00000000 ffffb481`7892d000 00000000`00000000 : nt!_chkstk+0x5d
fffff800`457730c4 : ffffb481`789326a8 ffffb481`789323f0 ffffb481`789326a8 ffffb481`78932570 : nt!KeQuerySystemTimePrecis
e+0x27d1
fffff800`457ee482 : 00003c74`00000000 fffff800`458a1d00 00000000`00000000 fffff800`45d940c4 : nt!KeQuerySystemTimePrecis
e+0x15f4
fffff800`457eafc0 : 00000000`00000000 fffff800`45a97fe0 ffff8301`2c077220 ffffc582`1bb72c30 : nt!setjmpex+0x7622
fffff806`f7e010e6 : 00000000`00000001 00000000`00000000 ffffb481`76e2e000 fffff800`456e6511 : nt!setjmpex+0x4160
fffff806`f7e07020 : ffffc582`1bb72c30 ffffc582`19f18000 ffffc582`1bb72c30 ffff3ac8`f399d666 : ddapmcrash+0x10e6
fffff800`45b338f7 : 00000000`00000000 00000000`00000000 ffffc582`1bb72c30 ffffffff`000001c8 : ddapmcrash+0x7020
fffff800`45ad140e : 00000000`00000000 00000000`00000000 00000000`00000000 fffff800`45a3e2c0 : nt!FsRtlNotifyVolumeEventE
x+0x243b
fffff800`45715dc9 : fffff800`00000000 ffffffff`80000ba4 ffffc582`1b4e3800 fffff800`45a3e2c0 : nt!MmGetPhysicalMemoryRang
esEx+0xb56
fffff800`456c6f85 : ffffc582`1b4e3800 00000000`00000080 ffffc582`18a636c0 ffffc582`1b4e3800 : nt!KdPollBreakIn+0x8059
fffff800`457e4df6 : ffffb481`76e15180 ffffc582`1b4e3800 fffff800`456c6f44 00000000`00000246 : nt!PsGetProcessSessionIdEx
+0x2d5
00000000`00000000 : ffffb481`78933000 ffffb481`7892d000 00000000`00000000 00000000`00000000 : nt!KeSynchronizeExecution+
0x7756
PS C:\Users\Administrator>