rule FakeInstaller_Python_Fiber_MemoryLoader_Hunt
{
    meta:
        description = "Hunting rule for Python-based fake installer loaders that execute shellcode via fibers"
        author = "SixHands"
        date = "2026-04-21"
        yarahub_uuid = "54048355-7629-4243-9c60-7af4b7f99f33"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a6c4948ec670f6562d7007c3a6f1ec2b"

    strings:
        $a1 = "VirtualAlloc" ascii wide
        $a2 = "CreateFiber" ascii wide
        $a3 = "SwitchToFiber" ascii wide
        $a4 = "pythonw.exe" ascii wide
        $a7 = "ctypes.windll.kernel32.VirtualAlloc" ascii
        $a8 = "ctypes.windll.kernel32.SwitchToFiber" ascii

    condition:
        uint16(0) == 0x5A4D and 4 of them
}