rule Phish_VN_TechSupport_WebsiteDown_Kit {
    meta:
        description = "Kit phishing tech-support vietnamien : fausse page 'site hors service' a arc-en-ciel (BazaCall-like)"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / cluster HTML scam"
        confidence  = "high"
        yarahub_uuid            = "e69984f8-e3bf-4c3b-a3ca-7fce29c16181"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "2b6dcb7865fa21c057c9e1c515e0a93a"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Website hi" ascii wide nocase
        $b = "conic-gradient" ascii wide
        $c = ".rainbow" ascii wide
    condition:
        $a and 1 of ($b, $c)
}
