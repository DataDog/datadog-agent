rule MAC_Phish_Trezor_Start_Downloader {
    meta:
        description = "Chargeur macOS masque en app legere, contacte un domaine .space avec le chemin /trezor/start/ (appat wallet crypto Trezor)"
        author      = "Marjoriefort"
        date        = "2026-09-19"
        reference   = "Grand Scan InTheWild.0440 / miss 3dafc00c"
        yarahub_reference_md5 = "c1cdb1625b98d2bf531971ea8bd2637f"
        confidence  = "medium"
        yarahub_uuid = "8e159aa1-e21b-46ba-b068-356c9fca819d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "/trezor/start/" ascii wide
        $b = "setValue:forHTTPHeaderField:" ascii wide
    condition:
        $a and $b and (uint32(0) == 0xBEBAFECA or uint32(0) == 0xCFFAEDFE)
}
