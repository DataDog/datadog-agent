rule JAVA_Sample_Unique_2f9d9bc4
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "2f9d9bc4d40f64af3f401f7a0f05b7e980f43bd97b8b3994e2a2409a2b8eaddd"
        yarahub_uuid = "c19264a0-aff5-44a1-9ddd-d334902c76fb"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f149a5c7bf063d844504c1de124f8095"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/utils/hud/SpotifyBoard$Track" ascii
        $s1 = "com/nnpg/glazed/utils/hud/SpotifyBoard" ascii
        $s2 = "1(Lcom/nnpg/glazed/utils/hud/SpotifyBoard$Track;)I" ascii
        $s3 = "durationSeconds" ascii
        $s4 = "title;artist;positionSeconds;durationSeconds;playing;musicMs" ascii
        $s5 = "positionSeconds" ascii
        $s6 = "Not Playing" ascii
        $s7 = "Open Spotify" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
