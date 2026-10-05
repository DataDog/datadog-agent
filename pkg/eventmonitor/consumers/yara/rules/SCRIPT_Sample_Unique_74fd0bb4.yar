rule SCRIPT_Sample_Unique_74fd0bb4 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "74fd0bb45d835e60419dfac50d2fb2361b694b37d0d397b735880a39baddadaa.vbe"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6df8d5af-b9a4-446a-b1c8-f3a52baef833"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "67ee6305e9d1017148aa909f0c29391c"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    executeCommand = \"powershell.exe -nop -ep bypass -file \"\"\" & scriptPath & \"\"\"\"" ascii wide
        $b = "        result = result & Mid(charset, Int(Rnd * Len(charset)) + 1, 1)" ascii wide
    condition:
        all of them
}
