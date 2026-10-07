rule LNK_Kimsuky_PowerShell_Payload_Dropper {
    meta:
        description = "Detects Kimsuky LNK files containing obfuscated PowerShell byte-array decoding routines and payload drop/execution logic."
        author = "Serhii Kocherhan"
        date = "2026-09-20"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "791d0c0e-2f66-4595-8c1c-60fcbbd1d9b6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1523a2fcc901965ab4568d9fe829e4af"
        yarahub_reference_link = "https://wezard4u.tistory.com/429876"

    strings:
        // LNK File Header Magic
        $lnk_magic = { 4C 00 00 00 01 14 02 00 }

        // Generic pattern for array parsing and byte casting loop: .Add([byte] $var)
        $re_byte_cast = /\[void\]\$[a-zA-Z0-9_]+\.Add\(\[byte\]\s*\$[a-zA-Z0-9_]+\)/ ascii wide nocase

        // Generic pattern for key-offset subtraction math logic: $pw_num + 103 / $pw_num - $var
        $re_math_logic = /\$[a-zA-Z0-9_]+\s*=\s*\$[a-zA-Z0-9_]+\[\$[a-zA-Z0-9_]+\]\s*\+\s*103/ ascii wide nocase

        // PowerShell string-to-bytes conversion string
        $s_encoding = "[System.Text.Encoding]::UTF8.GetBytes(" ascii wide nocase

        // File drop and execution pipeline indicators
        $s_out_file = "|Out-File -FilePath" ascii wide nocase
        $s_ps_exec = "Invoke-Expression" ascii wide nocase

    condition:
        $lnk_magic at 0 and
        filesize < 1MB and
        (
            ($re_byte_cast and $re_math_logic) or
            ($s_encoding and $s_out_file and $s_ps_exec and ($re_byte_cast or $re_math_logic))
        )
}