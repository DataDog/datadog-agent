rule lockbitblack_ransomnote {
    meta:
        date = "2022-07-02"
        description = "Hunting rule for LockBit Black/3.0 ransom notes"
        yarahub_author_twitter = "@captainGeech42"
        yarahub_uuid = "cc2308df-9b42-4169-8146-c63b0bc6b1f7"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "954d81de1c53158b0050b38d4f4b4801"
    strings:
        $s1 = "~~~ LockBit 3.0" ascii wide
        $s2 = "the world's fastest and most stable" ascii wide
        $s3 = "http://lockbitapt" ascii wide
        $s4 = ">>>>> Your data is stolen and encrypted" ascii wide
    condition:
        filesize < 20KB and 2 of them and #s3 > 10
}