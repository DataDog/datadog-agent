rule MaksStealer_Loader {
  meta:
    author = "ShadowOpCode"
    description = "Detects MaksStealer dropper/loader JAR"
    last_modified = "2025-05-18"
    date = "2025-08-19"
    yarahub_uuid = "37ece914-8bcd-4c6f-931b-9d42de974055"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "00000000000000000000000000000000"

  strings:
    $s0 = "MaxCoffe" ascii nocase

  condition:
    uint16be(0) == 0x504B and
    $s0
}