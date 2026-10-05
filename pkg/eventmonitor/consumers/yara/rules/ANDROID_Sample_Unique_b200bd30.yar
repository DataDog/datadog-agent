rule ANDROID_Sample_Unique_b200bd30 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "b200bd309b888d98faaac96f24ae5ea15a775fa4c88b829b3eedc7b678ddb4c7.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6abfb355-e7d9-468a-b1d8-4e0fdc1c916e"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "101a6d4d1a6aa9e7a231afa7d135dd38"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "META-INF/jars/commons-logging-1.3.5.jar" ascii wide
        $b = "META-INF/jars/httpcore-4.4.16.jar" ascii wide
    condition:
        all of them
}
