rule smokedham_installer {
  meta:
    date = "2025-05-12"
    target_entity = "file"
    yarahub_uuid = "aab6710b-9c4a-4f82-ba7d-27fcabb37f86"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "b587a6af7fd86eeb42425913b8d73d47"
  strings:
    $string1 = "VirtManage Pro" wide ascii
    $string2 = "NullsoftInst" wide ascii
  condition:
    uint16(0) == 0x5A4D and all of them
}
