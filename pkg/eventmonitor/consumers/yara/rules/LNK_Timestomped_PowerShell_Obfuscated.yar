rule LNK_Timestomped_PowerShell_Obfuscated {
    meta:
        description = "Detects timestomped LNK files with 1970 timestamps, zeroed volume serial, msedge icon, and obfuscated PowerShell arguments"
        author = "Serhii Kocherhan"
        date = "2026-09-03"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "53fa78e8-1e68-4127-8aee-525676d1d0f3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6e2c49f48be8f9ebc65e345d6a09e49d"

    strings:
        // Standard Windows Shell Link Header (HeaderSize: 0x4C, LinkCLSID: 00021401-0000-0000-C000-000000000046)
        $lnk_magic = { 4C 00 00 00 01 14 02 00 00 00 00 00 C0 00 00 00 00 00 00 46 }

        // FILETIME 0x019DB1DED53E8000 = 1970-01-01 00:00:00 UTC (Little Endian)
        $ft_1970 = { 00 80 3E D5 DE B1 9D 01 }

        // Target Working Directory & Local Path References
        $path_ps1 = "WindowsPowerShell\\v1.0" ascii wide nocase
        $path_pdata = "C:\\ProgramData" ascii wide nocase
        $icon_edge = "msedge.exe" ascii wide nocase

        // Obfuscated PowerShell Command Line Argument Signatures
        $arg_nologo = "-nOlOgo" ascii wide nocase
        $arg_nonint = "-NONinTErAC" ascii wide nocase
        $arg_nop    = "-nOP" ascii wide nocase
        $arg_env    = "$Env:COmSpeC[" ascii wide nocase

    condition:
        // Must begin with valid LNK Header Magic
        $lnk_magic at 0 and filesize < 2MB and
        (
            // Check for 1970 FILETIME at CreationTime (0x1C), AccessTime (0x24), and WriteTime (0x2C)
            $ft_1970 at 0x1C and
            $ft_1970 at 0x24 and
            $ft_1970 at 0x2C
        ) and
        (
            // Match obfuscated PowerShell argument structure
            ($arg_env or (2 of ($arg_nologo, $arg_nonint, $arg_nop))) and
            // Match associated metadata paths
            1 of ($path_ps1, $path_pdata, $icon_edge)
        )
}