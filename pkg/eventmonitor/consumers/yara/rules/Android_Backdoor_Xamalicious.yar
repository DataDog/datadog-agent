rule Android_Backdoor_Xamalicious
{
    meta:
        description = "Detects Xamalicious Android malware samples based on the strings matched"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        source = "https://www.mcafee.com/blogs/other-blogs/mcafee-labs/stealth-backdoor-android-xamalicious-actively-infecting-devices/"
        hash = "7149acb072fe3dcf4dcc6524be68bd76a9a2896e125ff2dddefb32a4357f47f6"
        date = "2024-01-26"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "76100929a9bad1da1d9421a91980a4b3"
        yarahub_uuid = "927341a0-9103-482a-9a95-10cbb6c7ae23"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $anrd = "AndroidManifest.xml"

        $xa1 = "xamarin_essentials_fileprovider_file_paths" 
        $xa2 = "Xamarin.Android v9.0 Support"
        $xa3 = "Xamarin.Android.Build.Tasks"
        $xa4 = "Xamarin.Forms.Platform.Android"
        $xa5 = "Xamarin.Android v7.0 Support"
        $xa6 = "Xamarin.Forms"
        $xa7 = "com.xamarin.formsviewgroup" nocase
        $xa9 = "com/xamarin/formsviewgroup/BuildConfig"
        $xa10 = "Xamarin.Essentials"

        $mcrf1 = "Microsoft.AspNetCore.Http.HttpResponse"
        $mcrf2 = "Microsoft.AspNetCore.Http.HttpRequest"
        $mcrf3 = "Microsoft.AspNetCore.Http.HttpContext"
        $mcrf4 = "Microsoft.AspNetCore.Builder.IApplicationBuilder"

        $microsoft = "Microsoft"

        $per1 = "android.permission.BIND_JOB_SERVICET"
        $per2 = "android.permission.WRITE_EXTERNAL_STORAGE"
        $per3 = "android.permission.INTERNET"

        $wid1 = "Xamarin.Android bindings for Android Support Library - runtime" wide
        $wid2 = "Xamarin.Android.Arch.Core.Runtime" wide
        $wid3 = "Xamarin.Android.Arch.Core.Runtime.dll" wide

        $wide1 = "com/xamarin/forms/platform/android/FormsViewGroup" wide
        $wide2 = "com/xamarin/forms/platform/android" wide
        $wide3 = "com/xamarin/formsviewgroup/BuildConfig" wide
        $wide4 = "com.xamarin.formsviewgroup" wide 
    
        $int1 = "android.hardware.display.category.PRESENTATION" wide
        $int2 = "android.intent.category.LEANBACK_LAUNCHER" wide
        $int3 = "android.intent.extra.HTML_TEXT" wide
        $int4 = "android.intent.extra.START_PLAYBACK" wide
        $int5 = "android.activity.usage_time" wide
        $int6 = "android.usage_time_packages" wide
        $int7 = "android.support.PARENT_ACTIVITY" wide

        
    condition:
        $anrd
        and 4 of ($xa*)
        and 2 of ($mcrf*) or $microsoft 
        and 1 of ($per*)
        and 2 of ($wid*)
        and 2 of ($wide*)
        and 4 of ($int*)
}
