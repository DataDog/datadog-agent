rule privateloader
{
    meta:
        author                    = "andretavare5"
        description               = "Detects PrivateLoader malware."
        org                       = "Bitsight"
        date                      = "2024-08-08"
        sample1_md5               = "8f70a0f45532261cb4df2800b141551d" // loader module Jan 2022
        sample2_md5               = "dbf48bf522a272297266c35b965c6054" // service module Nov 2023
        sample3_md5               = "51bb70b9a31d07c7d57da0c5b26545d4" // core module Dez 2023
        sample4_md5               = "3fe6f262a34d82a4cc96540e4105b581" // core module Jul 2024
        yarahub_author_twitter    = "@andretavare5"
        yarahub_reference_link    = "https://www.bitsight.com/blog/hunting-privateloader-malware-behind-installskey-ppi-service"
        yarahub_malpedia_family   = "win.privateloader"
        yarahub_uuid              = "5916c441-16b1-42b7-acaa-114c06296f38"
        yarahub_license           = "CC BY-NC-SA 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "3fe6f262a34d82a4cc96540e4105b581"

    strings:
        $hdr   = "Content-Type: application/x-www-form-urlencoded" wide ascii
        $dom1  = "ipinfo.io" wide ascii
        $dom2  = "db-ip.com" wide ascii
        $dom3  = "maxmind.com" wide ascii
        $dom4  = "ipgeolocation.io" wide ascii
        $ua1   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36" wide ascii
        $ua2   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/93.0.4577.63 Safari/537.36" wide ascii
        $ua3   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36" wide ascii
        $ua4   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" wide ascii

        // str decrypt
        // PXOR XMM(1/0)
        $asm1 = {66 0F EF (4?|8?)}

        // str decrypt
        // LEA ??
        // XOR ??
        // INC ??
        // CMP ??
        // JC ??
        $asm2 = {8D ?? ?? 30 ?? ?? ?? 4? FF C? 4? 83 F? ?? 72 ??}

        // str decrypt
        // LEA ??
        // INC ??
        // XOR ??
        // CMP ??
        // JC ??
        $asm3 = {8D ?? ?? 4? 30 ?? 83 F? ?? 72 ??}
                    
    condition:
        uint16(0) == 0x5A4D and filesize > 100KB and filesize < 10MB and $hdr and any of ($dom*) and any of ($ua*) and any of ($asm*)
}
