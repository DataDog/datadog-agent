rule SCRIPT_Downloader_Unknown_AVTECH_ConfigInject {
    meta:
        description = "Config camera/routeur (format AVTECH) avec injection downloader : wget http:// + chmod 7 + sh dans un champ Password"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "Veille glissante 2026-09-17_18 / grappe 8 configs"
        confidence  = "high"
        yarahub_uuid              = "c82445e3-7efe-43a8-b705-424140ef9fc2"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "f43802a407264f62b321d19e69ec6ad9"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "<Password Level=\"40/40\" Dispatch=\"account\">" ascii wide
        $b = "wget http://" ascii wide
        $c = "chmod 7" ascii wide
        $d = ";sh " ascii wide
    condition:
        $a and $b and $c and $d
}
