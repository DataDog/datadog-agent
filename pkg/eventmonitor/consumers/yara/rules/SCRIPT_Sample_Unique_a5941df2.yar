rule SCRIPT_Sample_Unique_a5941df2 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "a5941df2bbe68acd086890dbe82eec9dfc522ab3c8ee99f78ee6458c2fe09527.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "7bb0ffaa-4a3a-47dd-bd37-528b9821dc7d"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "22b95e19770740187724a86bea324cd8"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "5ecf5676c0e49c3b17b89bc6" ascii wide
        $b = "5ecf5676c0e49c3b17b89bc6" ascii wide
    condition:
        all of them
}
