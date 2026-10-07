rule Foudre_Backdoor {
   meta:
      description = "Detects Foudre Backdoor"
      author = "Sid"
      date = "2024-08-09"
      yarahub_uuid = "4c36d37f-9550-474e-aa55-fb154098462c"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "c45167396be510d5ee4da51ff7544d5e"

     strings:
      $s1 = "main.exe" fullword ascii
      $s2 = "pub.key" fullword ascii
      $s3 = "WinRAR self-extracting archive" fullword wide
   condition:
      ( uint16(0) == 0x5a4d and filesize < 2000KB and all of them )
}