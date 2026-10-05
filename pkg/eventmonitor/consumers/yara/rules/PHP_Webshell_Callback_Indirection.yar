rule PHP_Webshell_Callback_Indirection {
    meta:
        description = "PHP webshell using trait/class method indirection ($a($b) arbitrary call)"
        author = "Marjoriefort"
        date = "2026-09-13"
        reference = "InTheWild.0438 / tennc 006_php3"
        confidence = "high"
        yarahub_uuid            = "7c0f72aa-3ad5-4f83-9769-a3c2ece2bf38"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "b16224464fb01ff9115fb215137337fd"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "public function eat($a, $b)"
        $b = "$a($b);"
        $c = "This is dog drive"
    condition:
        all of them
}
