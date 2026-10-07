rule LNK_Timestomped_Hex_SelfSeeker_Payload {
    meta:
        description = "Detects timestomped LNK files using hex-encoded PowerShell to read/xor payload data appended to the LNK file"
        author = "Serhii Kocherhan"
        date = "2026-09-06"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "005716a2-6d97-4c54-a0db-c49a61c42db0"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c7723bf166ef08ff3112257a1244f584"

    strings:
        // Windows Shell Link Header (%LNK Magic)
        $lnk_magic = { 4C 00 00 00 01 14 02 00 00 00 00 00 C0 00 00 00 00 00 00 46 }

        // FILETIME 0x019DB1DED53E8000 = 1970-01-01 00:00:00 UTC (Little Endian)
        $ft_1970 = { 00 80 3E D5 DE B1 9D 01 }

        // Specific Command Line Argument & Execution Flags
        $arg_winstyle = "-WindowStyle Hidden" ascii wide nocase
        $arg_execbypass = "ExecutionPolicy Bypass" ascii wide nocase

        // Stager Hex Convert Routine Artifacts
        $hex_sub = "$hex.Substring(" ascii wide nocase
        $hex_convert = "[Convert]::ToByte(" ascii wide nocase
        $hex_var = "$hex = '" ascii wide nocase

        // Unique Hex Fragments from Decoded Stager (Get-Location, Seek, Read, bxor pattern)
        // Hex byte sequence matching "Get-Location; $S6" ("244E3858203D204765742D4C6F636174696F6E3B20245336")
        $hex_payload_fragment = "32303437363537343639364636453342" ascii wide nocase

        // Masquerading Icon Reference
        $icon_pdf = ".pdf" ascii wide nocase

    condition:
        // Verify standard LNK magic header and file size boundary
        $lnk_magic at 0 and filesize < 10MB and

        // Check for 1970 FILETIME at CreationTime (0x1C), AccessTime (0x24), and WriteTime (0x2C)
        $ft_1970 at 0x1C and
        $ft_1970 at 0x24 and
        $ft_1970 at 0x2C and

        // Core execution signature matches
        $arg_winstyle and
        $hex_var and
        all of ($hex_sub, $hex_convert) and
        (
            $arg_execbypass or $hex_payload_fragment or $icon_pdf
        )
}