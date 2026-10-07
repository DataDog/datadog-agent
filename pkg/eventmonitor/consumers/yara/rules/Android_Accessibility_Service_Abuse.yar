/*
    YARA Rule: Android Accessibility Service Abuse
    Target:    MalwareBazaar / YARAify (standard YARA, no external modules)
    Purpose:   Detect Android malware that abuses the Accessibility Service API
               to perform auto-clicking, credential theft, overlay attacks,
               keylogging, or unauthorized device control.

    Design rationale:
      - APK files are ZIP archives containing DEX bytecode where Java/Kotlin
        strings are stored as UTF-8 in the string table, making them directly
        searchable with standard YARA string matching.
      - AndroidManifest.xml is compiled to Android Binary XML (AXML), but its
        string pool also contains readable UTF-8 string literals.
      - Legitimate apps (password managers, screen readers) declare accessibility
        but rarely combine it with SMS interception, device admin, overlays,
        keylogging, and anti-analysis. The condition logic requires a quorum
        of malicious behavioral indicators alongside accessibility markers
        to minimize false positives on benign software.
*/

rule Android_Accessibility_Service_Abuse
{
    meta:
        description   = "Detects Android malware abusing Accessibility Service for auto-clicking, credential theft, overlay attacks, or device takeover"
        author        = "Buga :3"
        date          = "2026-02-14"
        reference     = "https://developer.android.com/reference/android/accessibilityservice/AccessibilityService"
        tlp           = "TLP:WHITE"
        target_entity = "file"

        // YARAhub metadata (optional, for deployment on YARAify)
        yarahub_author_twitter  = "@Bugamashoo"
        yarahub_reference       = "https://attack.mitre.org/techniques/T1629/001/"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5   = "23be84b5f050cb973e6d524beb39452d"
        yarahub_uuid            = "eb8be3a0-b01d-44af-87f1-16108865aad6"
        yarahub_license         = "CC0 1.0"

    strings:
        //--------------------------------------------------------------
        // Section 1: APK / Android structural markers
        //--------------------------------------------------------------
        $zip_header    = { 50 4B 03 04 }                       // PK ZIP local file header
        $dex_magic     = { 64 65 78 0A 30 }                    // "dex\n0" -- DEX file magic (covers 035-045)
        $manifest      = "AndroidManifest.xml"          ascii   // Must contain a manifest

        //--------------------------------------------------------------
        // Section 2: Accessibility Service declaration / usage
        //   At least 2 required -- filters out files that only
        //   coincidentally contain one of these strings.
        //--------------------------------------------------------------
        $acc_service   = "AccessibilityService"         ascii   // The base class or service type
        $acc_bind      = "BIND_ACCESSIBILITY_SERVICE"   ascii   // Permission in manifest
        $acc_event     = "onAccessibilityEvent"         ascii   // Core callback method
        $acc_info      = "AccessibilityServiceInfo"     ascii   // Service info / configuration
        $acc_config    = "accessibility_service_config"  ascii   // XML config resource reference
        $acc_node      = "AccessibilityNodeInfo"        ascii   // Node inspection (screen scraping)
        $acc_wchange   = "TYPE_WINDOW_STATE_CHANGED"    ascii   // Event type: window transitions
        $acc_cchange   = "TYPE_WINDOW_CONTENT_CHANGED"  ascii   // Event type: content changes

        //--------------------------------------------------------------
        // Section 3: Malicious behavioral indicators
        //   These are capabilities rarely combined with accessibility
        //   in legitimate apps. Banking trojans, RATs, and spyware
        //   chain these together with accessibility abuse.
        //--------------------------------------------------------------

        // --- Auto-clicking / gesture injection ---
        $mal_globalact = "performGlobalAction"          ascii   // Simulate BACK, HOME, RECENTS
        $mal_dispatch  = "dispatchGesture"              ascii   // Inject taps / swipes
        $mal_actclick  = "ACTION_CLICK"                 ascii   // Click a UI node
        $mal_actscroll = "ACTION_SCROLL_FORWARD"        ascii   // Auto-scroll
        $mal_perfact   = "performAction"                ascii   // Generic node action
        $mal_settext   = "ACTION_SET_TEXT"              ascii   // Inject text into fields

        // --- Keylogging via accessibility ---
        $mal_keyfilter = "FLAG_REQUEST_FILTER_KEY_EVENTS" ascii // Intercept keypresses
        $mal_keyevent  = "onKeyEvent"                   ascii   // Key event handler

        // --- Device admin abuse (prevent uninstall) ---
        $mal_devadmin  = "BIND_DEVICE_ADMIN"            ascii   // Device admin permission
        $mal_addadmin  = "android.app.action.ADD_DEVICE_ADMIN" ascii

        // --- Overlay / phishing injection ---
        $mal_overlay   = "TYPE_APPLICATION_OVERLAY"     ascii   // Draw-over-apps overlay
        $mal_sysalert  = "SYSTEM_ALERT_WINDOW"          ascii   // Overlay permission
        $mal_wmmgr     = "WindowManager$LayoutParams"   ascii   // Programmatic overlay setup

        // --- SMS/OTP interception (banking trojans) ---
        $mal_rcvsms    = "RECEIVE_SMS"                  ascii   // Intercept incoming SMS
        $mal_readsms   = "READ_SMS"                     ascii   // Read SMS messages
        $mal_sendsms   = "SEND_SMS"                     ascii   // Send SMS (premium fraud)
        $mal_smsrecv   = "SmsReceiver"                  ascii   // Common receiver class name

        // --- Package / app observation ---
        $mal_getpkg    = "getInstalledPackages"         ascii   // Enumerate installed apps
        $mal_pkgadd    = "PACKAGE_ADDED"                ascii   // Monitor app installs
        $mal_pkgname   = "packageName"                  ascii   // Target specific packages
        $mal_launcher  = "getLaunchIntentForPackage"    ascii   // Launch target apps

        // --- Anti-analysis / evasion ---
        $mal_emulator  = "isEmulator"                   ascii
        $mal_rooted    = "isDeviceRooted"               ascii
        $mal_goldfish  = "goldfish"                     ascii   // Emulator board name
        $mal_genymo    = "genymotion"                   ascii   // Genymotion emulator

        // --- C2 / data exfil indicators ---
        $mal_httppost  = "POST"                         ascii
        $mal_usragent  = "User-Agent"                   ascii
        $mal_jsonobj   = "JSONObject"                   ascii
        $mal_getimei   = "getDeviceId"                  ascii   // Steal IMEI
        $mal_getphone  = "getLine1Number"               ascii   // Steal phone number
        $mal_accounts  = "GET_ACCOUNTS"                 ascii   // Harvest accounts

        // --- Screen capture ---
        $mal_screencap = "MediaProjection"              ascii   // Screen recording API
        $mal_imgread   = "ImageReader"                  ascii   // Capture screen frames

    condition:
        // --- File must be an Android APK ---
        $zip_header at 0 and
        $dex_magic and
        $manifest and

        // --- Must declare/use Accessibility Service ---
        //     Require at least 2 distinct accessibility markers.
        2 of ($acc_*) and

        // --- Must exhibit malicious behavior ---
        //     Require at least 4 distinct malicious indicators.
        //     This threshold filters out legitimate apps that may
        //     use one or two of these APIs for valid reasons.
        4 of ($mal_*)
}
