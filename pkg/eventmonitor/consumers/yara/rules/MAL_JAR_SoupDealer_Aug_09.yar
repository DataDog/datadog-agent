rule MAL_JAR_SoupDealer_Aug_09 : JAR { 
meta:
  description = "This rule detects samples that use the SoupDealer loader."
  author      = "Utku Corbaci / Malwation"
  date        = "2025-08-09"
  sharing     = "TLP:WHITE"
  tlp         = "WHITE"
  tags        = "windows,jar,malicious,loader"
  sample      = "d286acf63f5846e775ba23599e2b5be88d0564d24f29e0646f6cff207249c130"
  reference   = "https://www.malwation.com/blog/technical-analysis-of-a-stealth-java-loader-used-in-phishing-campaigns-targeting-turkiye"
  reference2  = "https://app.threat.zone/submission/5488b11f-43fb-46c0-a0e5-6e27c9138195/overview"
  os          = "Windows"
  category    = "Malicious"
  yarahub_reference_md5 = "b67580eee2d359825cccc2f637794bf5"
  yarahub_author_twitter = "@rhotav"
  yarahub_author_email = "utku@rhotav.com"
  yarahub_reference_link = "https://www.malwation.com/blog/technical-analysis-of-a-stealth-java-loader-used-in-phishing-campaigns-targeting-turkiye"
  yarahub_uuid = "72ba21d3-4781-406d-93b7-b15308be456d"
  yarahub_license = "CC0 1.0"
  yarahub_rule_matching_tlp = "TLP:WHITE"
  yarahub_rule_sharing_tlp = "TLP:WHITE"

strings:
    // This strings specific to reduce false positives
    $manifest1 = "META-INF/MANIFEST.MFPK" fullword
    $manifest2 = "META-INF/MANIFEST.MFM" fullword

    $single_upper = /[A-Z]\.classPK/
    $res_pk = /[A-Za-z0-9]{11,}PK/ fullword

condition:
    uint32(0) == 0x04034b50 
    and all of ($manifest*)
    and (#single_upper == 2 or #single_upper == 5 or #single_upper == 11)
    and (#res_pk > 0)
}