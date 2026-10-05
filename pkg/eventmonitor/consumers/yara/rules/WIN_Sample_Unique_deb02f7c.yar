rule WIN_Sample_Unique_deb02f7c
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (pe, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "deb02f7c30d322c2003a0f4e29fbd08f172e8c7fd22b1ca47c6f77ebf1a7b914"
        yarahub_reference_md5 = "0876455207d7f1bbef39ea5b25a183c3"
        yarahub_uuid = "9596f6fb-fa43-4ece-95a1-7e7591aea3d5"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "pe"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.39"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=" ascii
        $s1 = "http://www.3721.com/adjusttime.htm" ascii
        $s2 = "SOFTWARE\\Classes\\MSipy" ascii
        $s3 = "Content-Transfer-Encoding:" ascii
        $s4 = "TIMPlatform.exe" ascii
        $s5 = "TIMP1atform.exe" ascii
        $s6 = "Software\\Classes\\MSipv" ascii
        $s7 = "Software\\Microsoft\\Windows\\CurrentVersion\\Run" ascii
        $s8 = "Software\\Microsoft\\Windows\\CurrentVersion\\RunServices" ascii
        $s9 = "Software\\Classes\\txtfile\\shell\\open\\command" ascii
        $s10 = "Software\\Classes\\exefile\\shell\\open\\command" ascii
        $s11 = "aha01%s@online.ie" ascii
        $s12 = "http://www.mxm9191.com/myrunner_up.exe" ascii
        $s13 = "http://www.zigui.org/article.php?id=103822" ascii
        $s14 = "SOFTWARE\\GoldenSoft\\Recovery Genius 21st" ascii
        $s15 = "SOFTWARE\\GoldenSoft\\Recovery Genius" ascii
        $s16 = "SOFTWARE\\YuanZhi\\Recovery Genius 21st" ascii
        $s17 = "SOFTWARE\\YuanZhi\\Recovery Genius" ascii
        $s18 = "xcept_handler3" ascii
        $s19 = "xcept_h*ndler3" ascii
    condition:
        uint16(0) == 0x5A4D and filesize < 50MB and 3 of ($s0, $s2, $s3, $s4, $s5, $s6, $s7, $s8, $s9, $s10, $s11, $s14, $s15, $s16, $s17, $s18, $s19) and 1 of ($s1, $s12, $s13)
}
