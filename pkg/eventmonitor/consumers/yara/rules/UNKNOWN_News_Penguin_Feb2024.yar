
rule UNKNOWN_News_Penguin_Feb2024 {
    meta:
        Description = "Detects an unknown File Type that was part of the tooling used by News Penguin to target orgs in Pakistan"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Credits = "Is Now on VT! for notification of the malware sample"
        Reference = "https://blogs.blackberry.com/en/2023/02/newspenguin-a-previously-unknown-threat-actor-targets-pakistan-with-advanced-espionage-tool"
        Hash = "538bb2540aad0dcb512c6f0023607382456f9037d869b4bf00bcbdb18856b338"
        date = "2024-02-27"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "861b80a75ecfb083c46f6e52277b69a9"
        yarahub_uuid = "45cc6729-fe81-4055-ba74-40f5a17d4fae"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

strings:
    $penguin = "penguin"
    condition:
        #penguin > 100       
     
 }

 

 