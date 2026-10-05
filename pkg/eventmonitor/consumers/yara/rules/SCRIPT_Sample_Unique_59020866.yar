rule SCRIPT_Sample_Unique_59020866 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "5902086654eba2f50b5253f5b2bf4fa561cd6e3b7ef94be5578918e9645592af.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6d39e932-7fd9-471a-9785-166685233a96"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "445d8466b83e5dee8c20989b007cca48"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_8145c2365a'" ascii wide
        $b = "return 'REDIS_OK_8145c2365a'" ascii wide
    condition:
        all of them
}
