rule LIN_Downloader_Mirai_Style {
    meta:
        description = "Script shell downloader style Mirai : curl C2/bins/... | bash, par architecture"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Formation veille 2026-09-16 / cluster cirqueira"
        confidence  = "high"
        yarahub_uuid            = "870b79bf-ad5b-4e50-924e-0cd1306d4039"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "3f5345346943103d88f8fbddc5fe2309"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "curl -s http://" ascii wide
        $b = "/bins/" ascii wide
        $c = "| bash" ascii wide
        $d = "Payload specifique pour architecture:" ascii wide
    condition:
        3 of ($a,$b,$c,$d)
}
