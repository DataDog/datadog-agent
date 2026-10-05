rule SUSP_JS_HexObfuscated_ActiveX_Launcher {
    meta:
        description = "JS obfusque (identifiants _0x) instanciant ActiveX sous WScript - dropper JScript frais"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / cluster JS _0x"
        confidence  = "medium-high"
        yarahub_uuid            = "aab851a8-8138-4095-bdef-6ca48878c279"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "e847743b66dddcfc3afe248a42a71928"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "WScript" ascii wide
        $b = "ActiveXObject(_0x" ascii wide
    condition:
        all of them
}

