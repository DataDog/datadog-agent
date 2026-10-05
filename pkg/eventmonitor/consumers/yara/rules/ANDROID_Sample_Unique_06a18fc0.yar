rule ANDROID_Sample_Unique_06a18fc0 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "06a18fc06425111a95fc85c16ff679538a66c9fed00c059261856c519cb1ad73.jar"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "ec8ded0a-c44c-463a-b209-34dd414fc798"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "aa97f1e328a9ca5bdcca53972843df6b"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "E\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\\" ascii wide
        $b = "META-INF/native-image/okhttp/okhttp/native-image.propertiesU" ascii wide
    condition:
        all of them
}
