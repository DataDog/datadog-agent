rule ANDROID_Sample_Unique_32076454 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "32076454e6bcae047a7c477696af37178475280b2c4f32cc3c3c59e58b37fb08.apk"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "c1134bc3-a7b3-457b-a6ae-0ae0fd4962b2"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "de311e49fd9bf90f6b4eb429ea2e2ffb"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "~~Suchen Sie \"%1$s\" im Bereich \"Dienste\" und aktivieren Sie den Schalter. Die Einrichtung wird danach automatisch abgeschlossen." ascii wide
        $b = "HHTippen Sie auf \"%1$s\" in der Dienstliste und aktivieren Sie den Schalter" ascii wide
    condition:
        all of them
}
