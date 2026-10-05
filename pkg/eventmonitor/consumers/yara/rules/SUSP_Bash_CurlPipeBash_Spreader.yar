rule SUSP_Bash_CurlPipeBash_Spreader {
    meta:
        description = "Spreader bash minimaliste (curl | bash)"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / spreader"
        confidence  = "medium"
        yarahub_uuid            = "da7783b2-e0cd-4b2c-a38f-d2396f7dbed4"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "5b5c173657ad1baa068e171eac5be389"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Spread attempt complete" ascii wide
        $b = "curl -s http://" ascii wide
    condition:
        all of them
}

