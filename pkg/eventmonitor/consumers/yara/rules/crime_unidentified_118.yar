rule crime_unidentified_118 {
  meta:
    author = "kevoreilly"
    date = "2024-12-23"
    description = "Detects malware family unidentified_118"
    malpedia_family = "win.unidentified_118"
    modified = "2024-12-23"
    status = "RELEASED"
    version = "1.0"
    yarahub_author_twitter = "@capesandbox"
    yarahub_license = "CC BY-SA 4.0"
    yarahub_reference_link = "https://x.com/naumovax/status/1841092516302504155"
    yarahub_reference_md5 = "060c37258688dcda94ba09e88ece1af0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_uuid = "292a0081-cfea-4774-9c1b-8dbcb8b3fea3"

  strings:
    $syscall = { 48 31 C0 4C 8B 19 8B 41 10 48 8B 49 08 49 89 CA 41 FF E3 }

  condition:
    $syscall
}
