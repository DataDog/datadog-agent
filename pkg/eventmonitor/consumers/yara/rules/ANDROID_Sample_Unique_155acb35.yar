rule ANDROID_Sample_Unique_155acb35 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "155acb35cece5a4df8255853905e6136c5af5ae8725110c8cfe37786126dd994.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "3084e079-fb8b-4eab-8a5d-82f0cf0779b5"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "55a2845290901bc99f9a756adeae314a"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "base-debug.accesswidenerKLNN-." ascii wide
        $b = "base-debug.mixins.jsonPK" ascii wide
    condition:
        all of them
}
