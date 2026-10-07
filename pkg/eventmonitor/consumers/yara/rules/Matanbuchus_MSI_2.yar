rule Matanbuchus_MSI_2 : matanbuchus msitwo
{
    meta:
        author                    = "Andre Gironda"
        date                      = "2022-06-16"
        description               = "Matanbuchus MSI contains CAB with DLL via Zip via HTML Smuggling via Zip as malspam attachment / TA570 who normally delivers Qakbot"
        hash                      = "5dcbffef867b44bbb828cfb4a21c9fb1fa3404b4d8b6f4e8118c62addbf859da"
        hash2                     = "4d5da2273e2d7cce6ac37027afd286af"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_twitter    = "@AndreGironda"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "4d5da2273e2d7cce6ac37027afd286af"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "f29897f3-a6f1-43d7-b1cf-553671dc3c75"
   strings:
        $hex_36855 = { 50 72 69 76 61 74 65 20 4f 72 67 61 6e 69 7a 61 74 69 6f 6e 31 }
        $hex_368bd = { 57 65 73 74 65 61 73 74 20 54 65 63 68 20 43 6f 6e 73 75 6c 74 69 6e 67 2c 20 43 6f 72 70 2e 31 }
    condition:
        all of them
}
