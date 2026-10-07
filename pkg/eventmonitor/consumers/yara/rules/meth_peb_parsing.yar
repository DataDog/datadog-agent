rule meth_peb_parsing {
  meta:
    date = "2022-06-13"
    author = "Willi Ballenthin"
    yarahub_author_email = "william.ballenthin@mandiant.com"
    yarahub_author_twitter = "@williballenthin"
    yarahub_uuid = "fc096806-e637-43ac-b969-ec6a1f37328a"
    yarahub_license = "CC BY 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "00000000000000000000000000000000"
    strings:
       //                                                         ;; TEB->PEB
       // (64 a1 30 00 00 00 |                                    ; mov eax, fs:30
       //  64 8b (1d | 0d | 15 | 35 | 3d) 30 00 00 00 |           ; mov $reg, DWORD PTR fs:0x30
       //  31 (c0 | db | c9 | d2 | f6 | ff) [0-8] 64 8b ?? 30 )   ; xor $reg; mov $reg, DWORD PTR fs:[$reg+0x30]
       // [0-8]                                                   ; up to 8 bytes of interspersed instructions
       //                                                         ;; PEB->LDR_DATA
       // 8b ?? 0c                                                ; mov eax,DWORD PTR [eax+0xc]
       // [0-8]                                                   ; up to 8 bytes of interspersed instructions
       //                                                         ;; LDR_DATA->OrderLinks
       // 8b ?? (0c | 14 | 1C)                                    ; mov edx, [edx+0Ch]
       // [0-8]                                                   ; up to 8 bytes of interspersed instructions
       //                                                         ;; _LDR_DATA_TABLE_ENTRY.DllName.Buffer
       // 8b ?? (28 | 30)                                         ; mov esi, [edx+28h]
       $peb_parsing = { (64 a1 30 00 00 00 | 64 8b (1d | 0d | 15 | 35 | 3d) 30 00 00 00 | 31 (c0 | db | c9 | d2 | f6 | ff) [0-8] 64 8b ?? 30 ) [0-8] 8b ?? 0c [0-8] 8b ?? (0c | 14 | 1C) [0-8] 8b ?? (28 | 30) }

       $peb_parsing64 = { (48 65 A1 60 00 00 00 00 00 00 00 | 65 (48 | 4C) 8B ?? 60 00 00 00 | 65 A1 60 00 00 00 00 00 00 00 | 65 8b ?? ?? 00 FF FF | (48 31 (c0 | db | c9 | d2 | f6 | ff) | 4D 31 (c0 | c9))  [0-16] 65 (48 | 4d | 49 | 4c) 8b ?? 60) [0-16] (48 | 49 | 4C) 8B ?? 18 [0-16] (48 | 49 | 4C) 8B ?? (10 | 20 | 30) [0-16] (48 | 49 | 4C) 8B ?? (50 | 60) }

    condition:
       $peb_parsing or $peb_parsing64
}