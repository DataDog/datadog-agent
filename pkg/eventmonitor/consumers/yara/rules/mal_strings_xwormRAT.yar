rule mal_strings_xwormRAT
{
    meta:
        author = "m4nbat"
        description = "rule designed to match strings cvommonly associated with the XWorm RAT"
        status = "experimental"
        date = "2024-04-30"
        yarahub_author_twitter = "@knappresearchlb"
        yarahub_reference_md5 = "6b438a52d60887a534e6e38f72ededff"
        sha256 = "e761f2d9049734373c12c97aa557183081403e792b40028c410e4a6c0646c2b8"
        yarahub_uuid = "78ef8d56-538d-4990-a42d-5fac4f9315a2"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.xworm"
    strings:
        $str = "pL8W93lpOxCMdF9oyd51SA==" ascii wide nocase
        $str2 = "duRbxJbQYQN8i0MjbaAeEw==" ascii wide nocase
        $ua1 = "Mozilla/5.0 (iPhone; CPU iPhone OS 11_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/11.0 Mobile/15E148 Safari/604.1" ascii wide nocase
        $ua2 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/60.0.3112.113 Safari/537.36" ascii wide nocase
        $http1 = "Content-length: 5235" ascii wide nocase
        $http2 = "POST / HTTP/1.1" ascii wide nocase
        $http3 = "http://ip-api.com/line/?fields=hosting" ascii wide nocase
        $persist1 = "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run" ascii wide nocase
        $ps1 = "-ExecutionPolicy Bypass Add-MpPreference -ExclusionProcess" ascii wide nocase
        $ps2 = "-ExecutionPolicy Bypass Add-MpPreference -ExclusionPath" ascii wide nocase
        $ps3 = "-ExecutionPolicy Bypass -File" ascii wide nocase
        $ps4 = "powershell.exe" ascii wide nocase
        $enum1 = "SELECT * FROM Win32_VideoController" ascii wide nocase
        $enum2 = "Select * from Win32_ComputerSystem" ascii wide nocase
        $enum3 = "Select * from AntivirusProduct" ascii wide nocase
    condition:
        all of ($str*) and 
        all of ($ua*) and
        all of ($http*) and
        $persist1 and 
        all of ($ps*) and
        all of ($enum*)

        }
