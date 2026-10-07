rule SCRIPT_Sample_Unique_a2172f40 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "a2172f40e99b78fde9980848b5eaa7d223f31c58eecbf6acca85939c9a3465ff.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "67e8f7f8-f8d0-434c-bdf9-7cfc88c03c4d"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "cfb0c79b1ca5f52d78f19793d47052cd"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_a7a74227c8'" ascii wide
        $b = "return 'REDIS_OK_a7a74227c8'" ascii wide
    condition:
        all of them
}
