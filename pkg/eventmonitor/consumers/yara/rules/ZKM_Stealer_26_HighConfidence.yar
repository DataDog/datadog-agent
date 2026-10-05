rule ZKM_Stealer_26_HighConfidence {
  meta:
    author = "Brian (Omaha) - victim forensic investigation"
    description = "Detects ZKM Stealer 26.0.0 via obfuscator class strings - high confidence"
    date = "2026-04-13"
    yarahub_reference_link = "https://tria.ge/260407-s8dpgahs5l/behavioral1"
    yarahub_reference_md5 = "71d8a02cb6d6f3f5cb071395572aa0b4"
    yarahub_uuid = "7a2e4b8c-1d5f-4e9a-b3c6-2f8d1a4e7b9c"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
  strings:
    $ob1 = "afefbubbfsexqcob"
    $ob2 = "sfofdqwnzaiujuml"
    $ver = "ZKM26.0.0"
  condition:
    ($ob1 and $ob2) or ($ver and any of ($ob*))
}
