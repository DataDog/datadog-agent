rule MAL_Trigon_Dropper {
    meta:
        description = "Trigon multi-stage malicious dropper - process injection, keylogging, screenshot capture"
        author = "jaszzz"
        date = "2026-05-22"
        version = "1.9"
        yarahub_uuid = "2817bfaf-0a3f-4935-946f-1d8b09b63fb6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "590ec655beb6fa291ea73e2b473eba8e"
        reference = "URL|trigon"
        mitre_attack = "T1055, T1497, T1056, T1113, T1547, T1027, T1573"
        severity = "Critical"
        note = "Trigon dropper with process injection into tasklist.exe and keylogging capabilities"

    strings:
        $t1 = "Trigon" nocase ascii
        $t2 = "Trigon_Evo" nocase ascii
        $t3 = ".taubndl" ascii
        $t4 = "checks-user-input" ascii
        $t5 = "detect-debug-environment" ascii

        $inject_1 = "WriteProcessMemory" fullword ascii
        $inject_2 = "CreateRemoteThread" fullword ascii
        $inject_3 = "VirtualAllocEx" fullword ascii
        $inject_4 = "QueueUserAPC" fullword ascii
        $inject_5 = "SetThreadContext" fullword ascii
        $inject_6 = "GetThreadContext" fullword ascii
        $inject_7 = "CreateToolhelp32Snapshot" fullword ascii
        $inject_8 = "OpenProcess" fullword ascii

        $keylog_1 = "GetAsyncKeyState" fullword ascii
        $keylog_2 = "GetKeyboardState" fullword ascii
        $keylog_3 = "GetKeyState" fullword ascii
        $keylog_4 = "MapVirtualKeyA" fullword ascii
        $keylog_5 = "ToAsciiEx" fullword ascii

        $screen_1 = "CreateCompatibleBitmap" fullword ascii
        $screen_2 = "BitBlt" fullword ascii
        $screen_3 = "GetDC" fullword ascii
        $screen_4 = "ReleaseDC" fullword ascii

        $persist_1 = "Software\\Microsoft\\Windows\\CurrentVersion\\Run" ascii
        $persist_2 = "RegSetValueExW" fullword ascii
        $persist_3 = "RegOpenKeyExW" fullword ascii
        $persist_4 = "RegQueryValueExW" fullword ascii

        $net_1 = "WSAStartup" fullword ascii
        $net_2 = "WSASend" fullword ascii
        $net_3 = "InternetOpen" fullword ascii
        $net_4 = "InternetConnect" fullword ascii
        $net_5 = "HttpOpenRequest" fullword ascii
        $net_6 = "HttpSendRequest" fullword ascii
        
        $k_pipe1 = "NtCreateNamedPipeFile" fullword ascii
        $k_pipe2 = "CreatePipe" fullword ascii
        
        $evade_1 = "IsDebuggerPresent" fullword ascii
        $evade_2 = "CheckRemoteDebuggerPresent" fullword ascii
        $evade_3 = "Sleep" fullword ascii
        $evade_4 = "GetTickCount" fullword ascii

        $enc1 = "AES-256" ascii
        $enc2 = "AES-GCM" ascii

    condition:
        uint16(0) == 0x5A4D and
        uint32(uint32(0x3C)) == 0x00004550 and
        filesize < 20MB and
        (
            $t1 or $t2 or $t3 or $t4 or $t5
            or
            (
                3 of ($inject_*) and 
                2 of ($keylog_*) and 
                2 of ($screen_*) and 
                2 of ($persist_*) and 
                2 of ($net_*) and 
                ($k_pipe1 or $k_pipe2) and 
                2 of ($evade_*) and 
                ($enc1 or $enc2)
            )
        )
}
