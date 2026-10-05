rule ZKM_Stealer_26_Network {
  meta:
    author = "Brian (Omaha) - victim forensic investigation"
    description = "Detects ZKM Stealer 26.0.0 via network infrastructure indicators"
    date = "2026-04-13"
    yarahub_reference_link = "https://tria.ge/260407-s8dpgahs5l/behavioral1"
    yarahub_reference_md5 = "71d8a02cb6d6f3f5cb071395572aa0b4"
    yarahub_uuid = "9c3f5e7a-2b4d-4f8b-a1c5-3e7d2b5f8a1c"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
  strings:
    $c2 = "swordfull.info"
  condition:
    any of them
}
