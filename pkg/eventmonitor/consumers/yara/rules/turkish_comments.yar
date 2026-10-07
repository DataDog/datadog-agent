rule turkish_comments {
    meta:
        description = "Detects Turkish comments"
        author = "evilcel3ri"
        date = "2024-04-15"
        yarahub_uuid = "6098f265-e475-406e-9852-9703c65ab39f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9448b9d4e259bbce256046d8f52e7642"

    strings:
        $a = {53 6372 6970 7420 6261 c59f 6172 c4b1 796c 6120 79 c3bc 6b 6c65 6e64 69}
        $b = {5363 7269 7074 2079 c3bc 6b6c 656e 6972 6b65 6e20 6269 7220 6861 7461 206f 6c75 c59f 7475}
        $c = {53 6372 6970 7420 7a61 7465  6e20 79 c3bc 6b 6c65 6e64 69}

    condition:
        any of them
}