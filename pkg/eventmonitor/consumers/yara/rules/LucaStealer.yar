rule LucaStealer {


   meta:
 
        author = "Chat3ux" 
        date = "2022-09-08" 
        yarahub_reference_md5 = "c73c38662b7283befc65c87a2d82ac94" 
        yarahub_uuid = "71c9c97e-161a-41c8-8014-4ee186c92a22" 
        yarahub_license = "CC0 1.0" 
        yarahub_author_twitter = "@Chat3ux_" 
        yarahub_rule_matching_tlp = "TLP:WHITE" 
        yarahub_rule_sharing_tlp = "TLP:WHITE"  
        description = "Lucasstealer"

   strings:

      $s1 = "passwords.txt" ascii wide
      $s2 = "cookies" ascii wide
      $s3 = "telegram" ascii wide
      $s4 = "sensfiles.zip" ascii wide
      $s5 = "screen-.png" ascii wide
      $s6 = "system_info.txt" ascii wide
      $s7 = "out.zip" ascii wide
      $s8 = "info.txt" ascii wide
      $s9 = "system_info.txt"
      $s11 = "dimp.sts"
      $s12 = "Credit Cards:"
      $s13 = "Wallets:"

   condition:
   ( 6 of ($s*) )
}