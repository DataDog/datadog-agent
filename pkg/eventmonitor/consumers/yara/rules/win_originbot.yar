rule win_originbot
{
  meta:
    author                    = "andretavare5"
    description               = "Detects OriginBot(net) / OriginLoader malware."
    org                       = "Bitsight"
    date                      = "2024-01-04"
    yarahub_license           = "CC BY-NC-SA 4.0"
    yarahub_uuid              = "e42f46dc-f20a-4f62-96bf-83e279749b99"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp  = "TLP:WHITE"
    yarahub_reference_md5     = "956e9017817d45887c738b82fdf47f4a"
    yarahub_reference_link    = "https://www.fortinet.com/blog/threat-research/originbotnet-spreads-via-malicious-word-document"
    yarahub_malpedia_family   = "win.originbot"
    yarahub_author_twitter    = "@andretavare5"

  strings:
    $str1 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:99.0) Gecko/20100101 Firefox/99.0" fullword wide ascii
    $str2 = "application/x-www-form-urlencoded" fullword wide ascii
    $str3 = "x-key" fullword wide ascii nocase
    $str4 = "POST" fullword wide ascii
    $str5 = "p=" fullword wide ascii
    $str6 = "TripleDES" fullword wide ascii
    $str7 = "downloadexecute" fullword wide ascii

  condition:
    uint16(0) == 0x5A4D and // MZ header
    filesize > 20KB and filesize < 500KB and
    all of them
}