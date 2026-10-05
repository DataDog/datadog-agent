rule RedLineStealerNyaxXCat: RedLineNyaxXCat 
{
   meta:
      description = "RedLineStealer"
      author = "Alex Necula"
      date = "2024-08-07"
      yarahub_reference_md5 = "731e0addfe7c32066783fa33db494eff"
      yarahub_uuid = "d5bab19f-7705-4665-8aa7-51c545c32294"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.redline_stealer"

   strings:
      $magic = { 4D 5A }
      $hex = { 00 50 72 6F 67 72 61 6D 99 4C 6F 61 64 65 72 00 4E 79 61 6E 00 }

   condition:
      $magic at 0 and $hex
}
