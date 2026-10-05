rule Win_FakeInstaller_PythonShellcodeLoader_Crepectl_2026
{
    meta:
        description = "Detects the analyzed fake installer sample using .key config, XOR key, and Python/fiber shellcode loader traits"
        author = "SixHands"
        date = "2026-04-21"
        yarahub_uuid = "3e83ae74-2508-4c27-ab08-24ad44c8133f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a6c4948ec670f6562d7007c3a6f1ec2b"

    strings:
        $xor_key = "TqDLmrBbmyw" ascii
        $pak_name = "resources.pak" ascii
        $exe_name = "crepectl.exe" ascii
        $orig_name = "pythonw.exe" wide ascii
        $api1 = "VirtualAlloc" ascii wide
        $api2 = "CreateFiber" ascii wide
        $api3 = "SwitchToFiber" ascii wide
        $node_loader = "ctypes.windll.kernel32.VirtualAlloc" ascii
        $node_loader2 = "ctypes.windll.kernel32.SwitchToFiber" ascii
        $key_struct = { 01 0D 00 0B 00 0C 00 72 65 73 6F 75 72 63 65 73 2E 70 61 6B 54 71 44 4C 6D 72 42 62 6D 79 77 63 72 65 70 65 63 74 6C 2E 65 78 65 }

    condition:
        (
            uint16(0) == 0x5A4D and
            3 of ($orig_name,$api1,$api2,$api3,$pak_name,$exe_name,$xor_key)
        )
        or $key_struct
        or
        (
            2 of ($node_loader,$node_loader2,$api1,$api2,$api3) and
            2 of ($pak_name,$exe_name,$xor_key)
        )
}