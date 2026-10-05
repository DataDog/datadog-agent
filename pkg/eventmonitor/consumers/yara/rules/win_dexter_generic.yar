rule win_dexter_generic {
    meta:
        author = "dubfib"
        date = "2025-02-08"
        malpedia_family = "win.dexter"

        yarahub_uuid = "6a8945cf-d271-463d-b42d-e6932f3edc8e"
        yarahub_reference_md5 = "7d08306e5a837245c3f343c73535afef"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_license = "CC BY 4.0"
        yarahub_reference_link = "https://github.com/dubfib/yara"

    strings:
        $str0 = "Mozilla/4.0(compatible; MSIE 7.0b; Windows NT 6.0)" fullword ascii
        $str1 = "WindowsResilienceServiceMutex" fullword ascii
        $str2 = "UpdateMutex:" fullword ascii
        $str3 = "NoProcess" fullword ascii
        $str4 = "gateway.php" fullword ascii
        $str5 = "/portal1/gateway.php" fullword ascii
        $str6 = "images/logo/header.php" fullword ascii
        $str7 = "SecureDll.dll" fullword ascii
        $str8 = "wuauclt.exe" fullword ascii
        $str9 = "wmiprvse.exe" fullword ascii
        $str10 = "alg.exe" fullword ascii
        $str11 = "C:\\Program Files\\Internet Explorer\\iexplore.exe" fullword wide
        $str12 = ".DEFAULT\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run" fullword ascii
        $str13 = "Software\\Microsoft\\Windows\\CurrentVersion\\Policies\\Associations" fullword ascii
        $str14 = "Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings\\Zones\\0" fullword ascii

    condition:
        uint16(0) == 0x5a4d and 
        5 of ($str*)
}