rule Android_ElectricityBill_Miner_APK
{
meta:
author = "ShriyaTiger"
description = "Detects Electricity Bill themed Android malware and Miner.apk payload"
date = "2026-06-16"


    yarahub_uuid = "b1d381f3-7d1d-42fb-827c-25a3c4e02857"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "a8a0b0942fc1d9b196f33ba231006b1b"

strings:

    /* Package Names */
    $pkg1 = "com.ykjlpy.alchemistdry"
    $pkg2 = "com.xhyvv.crhypktonym"

    /* Domain */
    $domain1 = "pool.jesfeoqrj3.xyz"
    $domain2 = "jesfeoqrj3.xyz"

    /* Native Libraries */
    $lib1 = "trimethylbenzene"
    $lib2 = "libluqioeogov.so"
    $lib3 = "libminer-arm32.so"

    /* Suspicious Files */
    $file1 = "wUiw0cinstaller.dex"
    $file2 = "wUiw0c2"
    $file3 = "w8VqtP18vNT83sNR"
    $file4 = "ONrK3KyI"
    $file5 = "output8.mp3"
    $file6 = "report.txt"

    /* Firebase */
    $firebase1 = "com.google.firebase.MESSAGING_EVENT"
    $firebase2 = "FirebaseMessagingKeepAliveService"

    /* Hidden Launcher */
    $hidden = "android.intent.category.INFO"

    /* Suspicious Permissions */
    $perm1 = "android.permission.ACCESS_SUPERUSER"
    $perm2 = "android.permission.REQUEST_INSTALL_PACKAGES"
    $perm3 = "android.permission.QUERY_ALL_PACKAGES"
    $perm4 = "android.permission.REQUEST_IGNORE_BATTERY_OPTIMIZATIONS"
    $perm5 = "android.permission.WAKE_LOCK"
    $perm6 = "android.permission.FOREGROUND_SERVICE"

    /* Network Infrastructure */
    $ip1  = "154.12.116.240"
    $ip2  = "163.70.144.35"
    $ip3  = "57.144.176.141"
    $ip4  = "172.217.194.5"
    $ip5  = "45.88.186.2"
    $ip6  = "31.57.63.5"
    $ip7  = "104.26.13.20"
    $ip8  = "18.140.250.18"
    $ip9  = "3.7.100.95"
    $ip10 = "45.88.186.244"
    $ip11 = "18.156.217.149"

condition:

    uint32(0) == 0x04034B50 and

    (
        any of ($pkg*) or
        any of ($domain*) or
        any of ($lib*) or
        any of ($file*) or
        any of ($firebase*) or
        2 of ($perm*) or
        $hidden or
        ($ip1 or $ip2 or $ip3 or $ip4 or $ip5 or
         $ip6 or $ip7 or $ip8 or $ip9 or $ip10 or $ip11)
    )


}
