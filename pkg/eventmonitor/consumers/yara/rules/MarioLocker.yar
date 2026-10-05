rule MarioLocker {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-08-24"
        description = "Detects MarioLocker Ransomware"
        yarahub_uuid = "b80e9415-6edc-4be9-a6d6-053b5eacc2af"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6f53f99b0a19150d53244d691dd04e80"
    
    strings:
        $RansomHouse = "Welcome to the RansomHouse" ascii
        $RansomNote = "How To Restore Your Files.txt" ascii
        $EncryptedFiles = "Encrypted files: %d" ascii
        $ext = ".mario" ascii

    condition:
        all of them and
        uint16(0) == 0x457f
}