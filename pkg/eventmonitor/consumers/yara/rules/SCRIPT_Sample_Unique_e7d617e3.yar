rule SCRIPT_Sample_Unique_e7d617e3 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "e7d617e3e81b7734e0e5bea2b130bca14cf8c1b75f88a01eb58e7e641a92cde7.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "eb4e0dbd-c995-4473-b28a-ff7e3ef4d277"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "5c18876954997e34210c830936dd26b4"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_4e2c5530ad'" ascii wide
        $b = "return 'REDIS_OK_4e2c5530ad'" ascii wide
    condition:
        all of them
}
