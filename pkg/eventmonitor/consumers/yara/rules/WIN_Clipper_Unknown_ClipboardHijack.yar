rule WIN_Clipper_Unknown_ClipboardHijack {
    meta:
        description = "DLL clipper (detournement presse-papiers crypto) : API clipboard completes + DLL compacte"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive 569 / cluster 282 DLL x 15360 o"
        confidence  = "high"
        yarahub_uuid            = "167f1163-e80b-483e-90f0-d2cdc431e6d9"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "114346affe021135d1c5c571b740ea64"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "OpenClipboard" ascii wide
        $b = "GetClipboardData" ascii wide
        $c = "SetClipboardData" ascii wide
        $d = "EmptyClipboard" ascii wide
        $e = "GetClipboardSequenceNumber" ascii wide
    condition:
        uint16(0) == 0x5A4D and filesize < 100KB and 4 of ($a, $b, $c, $d, $e)
}
