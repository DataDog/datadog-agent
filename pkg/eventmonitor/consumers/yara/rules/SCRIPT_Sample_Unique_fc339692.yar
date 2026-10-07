rule SCRIPT_Sample_Unique_fc339692 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "fc3396926afdafca5c05712462789274dac129e66d46c952c494a8b67cdc0340.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "bce84eb9-b317-4dbd-9b58-41b8c722a45a"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "c072e5f93bad8699f09eef0178b2f2ce"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_9825b77d31'" ascii wide
        $b = "return 'REDIS_OK_9825b77d31'" ascii wide
    condition:
        all of them
}
