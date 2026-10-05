rule SUSP_NET_Large_Static_Array_In_Small_File_Jan24 {
    meta:
        description = "Detects large static arrays in small .NET files "
        author = "Jonathan Peters"
        date = "2024-01-11"
        reference = "https://github.com/Workingdaturah/Payload-Generator/tree/main"
        hash = "7d68bfaed20d4d7cf2516c2b110f460cf113f81872cd0cc531cbfa63a91caa36"
        score = 60
        yarahub_uuid = "4c809450-46e2-45a8-9fe2-1c14796ffffa"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1e324da03ebebfec519c030040943be0"
    strings:
        $op = "{ 5F 5F 53 74 61 74 69 63 41 72 72 61 79 49 6E 69 74 54 79 70 65 53 69 7A 65 3D [6-10] 00 }"
    condition:
        uint16 ( 0 ) == 0x5a4d and filesize < 300KB and #op == 1
}