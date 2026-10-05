rule MAC_Loader_Osascript_ZipDropper {
    meta:
        description = "Chargeur macOS par AppleScript : drop /tmp/osalogging.zip et /tmp/test.scpt, execute via osascript, C2 /gate et /dynamic?txd="
        author      = "Marjoriefort"
        date        = "2026-09-19"
        reference   = "Grand Scan InTheWild.0440 / miss 4d751dd3"
        yarahub_reference_md5 = "f9e73c254d7d66e8a99daeb4462e8827"
        confidence  = "high"
        yarahub_uuid = "3acc0b88-fdb6-4320-99cf-cae753c86f71"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "/tmp/osalogging.zip" ascii wide
        $b = "/tmp/test.scpt" ascii wide
        $c = "osascript failed with status: %d" ascii wide
        $d = "/dynamic?txd=" ascii wide
        $e = "/gate" ascii wide
    condition:
        2 of ($a, $b, $c, $d, $e)
}
