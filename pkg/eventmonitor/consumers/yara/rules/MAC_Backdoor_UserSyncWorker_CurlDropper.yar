rule MAC_Backdoor_UserSyncWorker_CurlDropper {
    meta:
        description = "Backdoor macOS se faisant passer pour la tache 'UserSyncWorker' : telecharge un payload via curl, depose dans /tmp/run, log dans /Library/Logs"
        author      = "Marjoriefort"
        date        = "2026-09-19"
        reference   = "Grand Scan InTheWild.0440 / miss 06c74829"
        yarahub_reference_md5 = "aa3804744ef482e6c509a77511f98667"
        confidence  = "high"
        yarahub_uuid = "098a3479-9311-4f35-8c78-5a18771d9083"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "/Library/Application Support/UserSyncWorker" ascii wide
        $b = "/Library/Logs/UserSyncWorker.log" ascii wide
        $c = "/tmp/run" ascii wide
        $d = "%{http_code}" ascii wide
    condition:
        2 of ($a, $b, $c, $d)
}
