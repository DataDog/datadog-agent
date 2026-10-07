rule LummaInjector {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-08-18"
        description = "Detects LummaStealer injection into RegAsm.exe"
        yarahub_uuid = "c83b2373-4119-4a06-8c0b-af56a79e4f46"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "106317cd019b63fde3dc44b2e365d0e6"
        malpedia_family = "win.lumma"

    strings:
        $RegAsmPath = "QzpcXFdpbmRvd3NcXE1pY3Jvc29mdC5ORVRcXEZyYW1ld29ya1xcdjQuMC4zMDMxOVxcUmVnQXNtLmV4ZQ" wide ascii //Base64 encoded path to RegAsm.exe
        
        $CreateProcess = "CreateProcess" ascii // Spawns RegAsm.exe
        $VirtualAllocEx = "VirtualAllocEx" ascii // Allocate memory in RegAsm.exe
        $WriteProcess = "WriteProcessMemory" ascii // Injects into RegAsm.exe
        
    condition:
        all of them and
        uint16(0) == 0x5a4d
}