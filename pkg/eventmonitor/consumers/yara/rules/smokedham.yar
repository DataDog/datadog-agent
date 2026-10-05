rule smokedham {
  meta:
    date = "2025-05-12"
    target_entity = "file"
    yarahub_uuid = "c7c26314-8191-4b7b-bc0e-6eee0df7c5e3"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "c609dba29f9702442a0185576b777de3"
  strings:
    $string1 = "Microsoft\\WindowsUpdate24" wide ascii
    $string2 = "Microsoft\\LogUpdateWindows" wide ascii
    $string3 = "Microsoft\\UpdateDesktop\\UnicodeData" wide ascii
    $string4 = "$var4 = $var4_part0 + $var4_part1" wide ascii
  condition:
    any of them
}


