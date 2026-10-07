rule SCRIPT_Sample_Unique_21f211f4 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "21f211f46be6a37c136556e2a98b29200089a35a90f92c7e1f885371588a982a.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "29082a18-5b57-4eb7-bb7f-c0d9c830c8c5"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "0c9499969bd7c18fe79e47185e98d280"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "return 'REDIS_OK_c6ac47fabf'" ascii wide
        $b = "return 'REDIS_OK_c6ac47fabf'" ascii wide
    condition:
        all of them
}
