rule ANDROID_Sample_Unique_6fe3334c {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "6fe3334c4a75f0dc452e37d1b901591a3055802d09ca6c3557791871e877638e.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "8e6945ff-017b-4ceb-9153-acbda6115bc1"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "4718ad486f187484dcb48b6cd0bd41d3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "e7421c91-62b9-469c-ae5a-4635b551c8ef.binPK" ascii wide
        $b = "META-INF/jars/gson-2.10.1.jarPK" ascii wide
    condition:
        all of them
}
