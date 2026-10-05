rule SVCReady_Packed
{
    meta:
        author                    = "Andre Gironda"
        date                      = "2022-06-08"
        description               = "packed SVCReady / win.svcready"
        hash                      = "326d50895323302d3abaa782d5c9e89e7ee70c3a4fbd5e49624b49027af30cc5"
        hash2                     = "76d69ec491c0711f6cc60fbafcabf095"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "andreg@gmail.com"
        yarahub_author_twitter    = "@AndreGironda"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "76d69ec491c0711f6cc60fbafcabf095"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "db8e2535-efef-4ada-a67f-919970546b1e"
   strings:
        $hex_1003b3e0 = { 52 75 6e 50 45 44 6c 6c 4e 61 74 69 76 65 3a 3a 46 69 6c 65 20 68 61 73 20 6e 6f 20 72 65 6c 6f 63 61 74 69 6f 6e }
        $hex_1003b424 = { 50 61 79 6c 6f 61 64 20 64 65 70 6c 6f 79 6d 65 6e 74 20 66 61 69 6c 65 64 2c 20 73 74 6f 70 70 69 6e 67 }
        $hex_1003c234 = { 4e 6f 74 20 73 75 70 70 6f 72 74 65 64 20 72 65 6c 6f 63 61 74 69 6f 6e 73 20 66 6f 72 6d 61 74 20 61 74 20 25 64 3a 20 25 64 0a 00 5b 2d 5d 20 }
        $hex_1003c2cc = { 49 6e 76 61 6c 69 64 20 61 64 64 72 65 73 73 20 6f 66 20 72 65 6c 6f 63 61 74 69 6f 6e 73 20 62 6c 6f 63 6b }
   condition:
        all of them
}
