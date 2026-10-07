rule SCRIPT_Sample_Unique_b56cdf8e {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "b56cdf8e6080074fb1592e96e0bd2caa90a391c0933fae58e1e7f986986fd8ee.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "93c0a67d-2818-4722-8e83-fe1febbdfee9"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "e69a14e476bc0aca1b5274726f0fdb7a"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "e5f1c342da88022a530e64b9" ascii wide
        $b = "e5f1c342da88022a530e64b9" ascii wide
    condition:
        all of them
}
