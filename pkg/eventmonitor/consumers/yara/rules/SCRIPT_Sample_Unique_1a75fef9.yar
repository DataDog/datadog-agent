rule SCRIPT_Sample_Unique_1a75fef9 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "1a75fef91a93f610bd25b0587def046962456499845d188d0207976dd732492f.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "3f446129-e54c-44e8-a386-bba5579fa8b7"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "1bc0acd7ad02012cdf4390364d0f11bc"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "            <Permit8 Type=\"Channel\" Value=\"0,1,2..16\">PlaybackAudioStream</Permit8>" ascii wide
        $b = "            <Permit6 Type=\"Channel\" Value=\"0,1,2..16\">LiveAudioStream</Permit6>" ascii wide
    condition:
        all of them
}
