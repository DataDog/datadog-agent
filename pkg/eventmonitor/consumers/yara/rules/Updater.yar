rule Updater
{
    meta:
        description = "Detects a malware script with specific characteristics and strings such as Updater"
        author = "Malman"
        date = "2025-10-30"
        version = "1.0"
        yarahub_reference_md5 = "083668d72eab0f8f2f21522bf286a913"
        yarahub_uuid = "2c4e8a1f-3b7c-4d5e-9f6a-1b0d2a3c4e5f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $a = "logfile=C:\\wormlog.txt"
        $b = "targetdir=C:\\Users"
        $c = "encryptionkey=MySecretKey123"
        $d = "remoteaccessport=4444"
        $e = "remoteaccesspassword=Password123"
        $f = "ransomnote=C:\\ransom_note.txt"
        $g = "exfiltrationserver=http://64.246.123.125:3000/upload"
        $h = "keylogfile=C:\\keylogs.txt"
        $i = "credentialfile=C:\\credentials.txt"
        $j = "maliciouspayload=C:\\malicious_payload.exe"
        $k = "additionalpayload=C:\\additional_payload.exe"
        $l = "corruptionfile=C:\\corruption_data.bin"
        $m = "screenshotdir=C:\\screenshots"
        $n = "micrecordingsdir=C:\\micrecordings"
        $o = "webcamvideosdir=C:\\webcamvideos"
        $p = "additionalmaliciousfile=C:\\additional_malicious_file.exe"
        $q = "additionalcorruptionfile=C:\\additional_corruption_data.bin"
        $r = "additionalkeylogfile=C:\\additional_keylogs.txt"
        $s = "additionalcredentialfile=C:\\additional_credentials.txt"
        $t = "additionalransomnote=C:\\additional_ransom_note.txt"
        $u = "additionalexfiltrationserver=http://64.246.123.125:3000/upload"
        $v = "additionalmicrecordingsdir=C:\\additional_micrecordings"
        $w = "additionalwebcamvideosdir=C:\\additional_webcamvideos"
        $x = "certutil -encode"
        $y = "schtasks /create"
        $z = "powershell -Command"

    condition:
        10 of them
}
