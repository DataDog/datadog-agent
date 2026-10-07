rule ZKM_Stealer_26_Definitive {
  meta:
    author = "Brian (Omaha) - victim forensic investigation"
    description = "Detects ZKM Stealer 26.0.0 - Java RAT/Infostealer"
    date = "2026-04-13"
    yarahub_reference_link = "https://tria.ge/260407-s8dpgahs5l/behavioral1"
    yarahub_reference_md5 = "71d8a02cb6d6f3f5cb071395572aa0b4"
    yarahub_uuid = "3f8d2a1c-9b4e-4f7a-8c6d-1e5f2a3b4c7d"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
  strings:
    $ver  = "ZKM26.0.0"
    $ob1  = "afefbubbfsexqcob"
    $ob2  = "sfofdqwnzaiujuml"
    $dev1 = "com/mirac"
    $dev2 = "private-project"
    $wh   = "USER_WEBHOOK"
    $lic  = "license-202603171"
    $sel  = "selam"
    $c2   = "swordfull.info"
    $task = "App_root"
  condition:
    2 of them
}
