rule CVE_2017_17215 {
    meta:
	author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-27"
        description = "Detects exploitation attempt of CVE-2017-17215"
        yarahub_uuid = "bd62321c-ccb7-4d6b-b98a-740aec5a452c"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a051d2730d19261621bd25d8412ba8e4"
	yarahub_reference_link = "https://nvd.nist.gov/vuln/detail/CVE-2017-17215"

    strings:
        $uri = "/ctrlt/DeviceUpgrade" ascii
        $digest_auth = "Digest username=" ascii
        $realm = "realm=\"" ascii
        $nonce = "nonce=" ascii
        $response = "response=" ascii

    condition:
        all of them
}