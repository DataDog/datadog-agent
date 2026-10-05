rule RABBITHUNT_cls {
  meta:
    date = "2022-06-13"
    author = "Willi Ballenthin"
    yarahub_author_email = "william.ballenthin@mandiant.com"
    yarahub_author_twitter = "@williballenthin"
    yarahub_uuid = "d7c6a7d6-20d9-40d0-a63c-2c780bee821e"
    yarahub_license = "CC BY 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "22a968beda8a033eb31ae175b7e0a937"
  strings:
    $a = "k_3872.cls"
    $b = "c_2910.cls"
    $c = "MataNet"
    $d = { 76 55 82 F6 93 82 B2 C7 77 15 13 3E 72 80 D4 DD }
    $e = { 72 82 EE F1 F2 8F C2 72 87 99 A8 2A AA C7 44 79 }
  condition:
    any of them
}