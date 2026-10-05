rule MULTI_Dropper_Unknown_GitHubObfuscated_JAR {
    meta:
        description = "JAR dropper : classes obfusquees a noms random sous com/github/ (densite >= 4) - famille du resolver GitHub"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "Veille glissante 2026-09-17_22 / grappe 21 JAR"
        confidence  = "high"
        note        = "Prolonge la regle Java_GitHub_Resolver_Dropper (specimen unique) : ici la FORME (regex + densite) couvre les variants randomises."
        yarahub_uuid              = "da27cd92-1eea-418a-b11f-071574c5258a"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "fe327e91fb8557e6c35cc6fa8958bb15"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip      = { 50 4B 03 04 }
        $manifest = "META-INF/MANIFEST.MF"
        $gh       = /com\/github\/[A-Za-z0-9_$]{3,40}\.class/
    condition:
        $zip and $manifest and #gh >= 4
}
