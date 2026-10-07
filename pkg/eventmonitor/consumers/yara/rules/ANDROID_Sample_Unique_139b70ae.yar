rule ANDROID_Sample_Unique_139b70ae {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "139b70aee8e4aa8b0f7ed08410d831293a14fc029520a68afdd325091375f5aa.apk"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6508a003-442e-420c-a263-22fc1a2ebe6b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "bf16fd95dc3418165ae297ba478c1298"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "META-INF/SIGNFILE.RSA3hb" ascii wide
        $b = "META-INF/SIGNFILE.SFPK" ascii wide
    condition:
        all of them
}
