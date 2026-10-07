rule SCRIPT_Sample_Unique_9ec39f33 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "9ec39f330c99f78395b144093d51489fed6e0ed36bc1fe71786b9a1f92b0bef5.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "434d2846-b3f4-439f-987f-7f0277b4e7cb"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "819d69247dfa05add47585611c82fe95"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_f1e18b4ed4'" ascii wide
        $b = "return 'REDIS_OK_f1e18b4ed4'" ascii wide
    condition:
        all of them
}
