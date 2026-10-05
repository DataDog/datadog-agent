rule Icedid_Unpacked_in_Memory {
  meta:
      author = "0x0d4y"
      description = "This rule detects samples from the IcedID family unpacked in memory, identifying code reuse of key functions."
      date = "2024-01-09"
      score = 90
      reference = "https://0x0d4y.blog/icedid-technical-analysis/"
      yarahub_reference_md5 = "5692c5708c71d0916ca48662a7ea9caf"
      yarahub_uuid = "ea2f87b5-6267-4d61-a69a-fa461203a7ef"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.icedid"
    strings:
    $hardware_info_collect_code_pattern = { 
        B8 00 00 00 40 0F A2 89 06 0F B6 44 24 16 89 5E 04 89 4E 08 89 56 0C FF 74 24 28 50 0F B6 44 24 1F 50 0F B6 44 24 24 50 0F B6 44 24 29 50 0F B6 44 24 2E 50 0F B6 44 24 33 50 68 ?? ?? 40 00
        }
    $ksa_prga_pattern = { 
        51 51 53 55 56 8B EA 89 4C 24 10 33 D2 57 8B 7C 24 1C 8B C2 88 04 38 40 3D 00 01 00 00 72 F5 8A CA 8B DA 8B 44 24 14 0F B6 F2 8A 14 3B 8A 04 06 02 C2 02 C8 88 4C 24 13 0F B6 C9 8A 04 39 88 04 3B 8D 46 01 88 14 39 33 D2 8A 4C 24 13 F7 F5 43 81 FB 00 01 00 00 
        }
    $xor_operation_pattern = {
        FE C3 0F B6 DB 8A 4C 1C 14 0F B6 D1 02 C2 0F B6 C0 89 44 24 10 8A 44 04 14 88 44 1C 14 8B 44 24 10 88 4C 04 14 8A 44 1C 14 02 C2 0F B6 C0 8A 44 04 14 32 04 3E 88 07 
        }
    $related_string1 = "WinHttpConnect"
    $related_string2 = "VirtualAlloc"
    $related_string3 = "WriteFile"
    $related_string4 = "CreateFileA"
    $related_string5 = "lstrcpyA"
    $related_string6 = "ProgramData"
    $related_string7 = "c:\\Users\\Public\\"
    $related_string8 = "%0.2X%0.2X%0.2X%0.2X%0.2X%0.2X%0.8X"
    $related_string9 = "%0.2X%0.8X%0.8X"
    condition:
        uint16(0) == 0x5a4d and
        ($hardware_info_collect_code_pattern or
        $ksa_prga_pattern or
        $xor_operation_pattern) or
        8 of ($related_string*)
}