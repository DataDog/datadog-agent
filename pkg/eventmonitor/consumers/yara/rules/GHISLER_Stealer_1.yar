rule GHISLER_Stealer_1 : ghisler stealer spyware
{
    meta:
        author                    = "Andre Gironda"
        date                      = "2022-11-11"
        description               = "GHISLER Golang based GO Stealer , POST /sendlog to http port 5000 , Userid HTTP header"
        hash                      = "30c1f93a3d798bb18ef3439db0ada4e0059e1f6ddd5d860ec993393b31a62842"
        hash2                     = "82040e02a2c16b12957659e1356a5e19"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_twitter    = "@AndreGironda"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "82040e02a2c16b12957659e1356a5e19"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "49ce8292-4a72-42d1-ab38-cdc076ff503d"
   strings:
        $hex_45ef40 = { 83 ec 24 8b 5c 24 28 c7 44 24 20 ff ff ff ff 89 5c 24 1c 64 8b 0d 14 00 00 00 8b 89 00 00 00 00 8b 49 18 8b 89 c8 01 00 00 89 4c 24 18 c7 44 24 14 00 00 00 00 c7 44 24 10 }
        $hex_4022a0 = { 8b 6c 24 04 f7 c5 07 00 00 00 74 05 e8 af }
        $s1 = "SetWaitableTimer" 
        $s2 = "SwitchToThread"
        $s3 = "time.Time.date"
        $s4 = "time.now"
        $go = "vendor/golang.org/x/net/dns/dnsmessage/message.go"
        $user = "Userid:"
        $name = "GHISLER"
    condition:
        all of them
}
