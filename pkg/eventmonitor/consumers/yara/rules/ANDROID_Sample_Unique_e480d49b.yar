rule ANDROID_Sample_Unique_e480d49b {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "e480d49be67ec5db92ff2042fb303d0e605b9d8f693be88d254c62d3aae15ded.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "3fef9ffa-b4f6-4ff3-9d2c-94181c8c7ec9"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "afdf2dd39f4a4bb1805d5768f6cc04f0"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "META-INF/jars/commons-logging-1.3.5.jar" ascii wide
        $b = "META-INF/jars/httpcore-4.4.16.jar" ascii wide
    condition:
        all of them
}
