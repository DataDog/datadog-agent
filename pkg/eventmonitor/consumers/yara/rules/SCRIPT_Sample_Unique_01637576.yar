rule SCRIPT_Sample_Unique_01637576 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "016375769c3c8028456603a5e78a2305b4617304a001fcbdd2001330d135bef2.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "1d9d6d05-e649-4f70-8e7b-1123e6746d1d"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7d8f51e11e0ba03a87ad5e4ae9abc933"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_0e6400beae'" ascii wide
        $b = "return 'REDIS_OK_0e6400beae'" ascii wide
    condition:
        all of them
}
