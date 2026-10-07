rule SCRIPT_Sample_Unique_302cc77f {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "302cc77fa3fec03fa2eb12fed9bce69c1547523c0a90c06cc769fc5f61142720.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "b7642485-7491-474e-b888-773afb59dbaa"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "ab359473c0f4483ddd274a3b4b97c6c3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    $tp=([int]$hdr[28] -shl 24) -bor ([int]$hdr[29] -shl 16) -bor ([int]$hdr[30] -shl 8) -bor [int]$hdr[31]" ascii wide
        $b = "        [IO.File]::WriteAllText(\"$d\\cursor_access_token.txt\",$found[\"cursorAuth/accessToken\"])" ascii wide
    condition:
        all of them
}
