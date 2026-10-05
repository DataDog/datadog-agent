rule meth_get_eip {
  meta:
    date = "2022-06-13"
    author = "Willi Ballenthin"
    yarahub_author_email = "william.ballenthin@mandiant.com"
    yarahub_author_twitter = "@williballenthin"
    yarahub_uuid = "666bfd55-7931-454e-beb8-22b5211ab04f"
    yarahub_license = "CC BY 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "9727d5c2a5133f3b6a6466cc530a5048"
    strings:
       // 0:  e8 00 00 00 00          call   5 <_main+0x5>
       // 5:  58                      pop    eax
       // 6:  5b                      pop    ebx
       // 7:  59                      pop    ecx
       // 8:  5a                      pop    edx
       // 9:  5e                      pop    esi
       // a:  5f                      pop    edi
       $x86 = { e8 00 00 00 00 (58 | 5b | 59 | 5a | 5e | 5f) }

    condition:
       $x86
}