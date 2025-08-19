# Handling of counter strings and translations in windows

Windows counter classes and counter names are localized in Windows.  This presents an extra degree of difficulty in reporting performance counters on Windows platforms.  

### Storage

The strings for the counter classes and counter names are stored in the registry.  They are stored in the key `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Perflib`. Beneath that key is a key `009` (for English), and a second key `CurrentLanguage` which contains the strings for the currently enabled locale.  On an en-us locale, the two keys contain the same information.

The strings are stored as a `REG_MULTI_SZ`, which means it is a "C" language string; in fact, it is a list of NULL terminated strings, with the last element indicated by NULLNULL.  For example, the contents might be (shortened for clarity): `"1|N|1847|N|2|N|System|N|4|N|Memory|N||N|"` (where |N| is NULL) 

To find the string for the current locale, in this example "System", first traverse the list of english strings.  System has the index "2".  Then, find the string with the index "2" in the current language, and Voila.

### Duplication

Because each package that reports counters via the performance counter API registers its own strings, strings can be duplicated. For example, the string "Write Bytes/sec" appears 4 times in one sample machine.

In addition, each of those might be translated differently.  The translation to the locale "foo" might have one instance "Write Bytes/sec" translated to "FooWrite Bytes/sec", and a second translated to "FooWrite Bytes per second".

This is because the first might be from the "System" package, and the second might be from the "NTDS" package, and the translations were generated separately.

The result of this is that while you use the same English string for two counters
* \\NTDS(*)\Write Bytes/sec
* \\System(*)\Write Bytes/sec

On a "fooish" locale, the counters would be
* \\NTDS(*)\FooWrite Bytes/sec
* \\System(*)\FooWrite Bytes per second

And, there is no way to know in advance which string to use.  

Therefore, the (somewhat messy) algorithm in `MakeCounterPath()` is to look up the string in the string table, and find the counter name that works.  Using the "fooish" example above, that means
1. Look up the index for "Write Bytes/sec".  This returns two indices A and B
2. Retrieve the fooish string for index A, "FooWrite Bytes/sec"
3. Attempt to create the counter "\\System(*)\FooWrite Bytes/sec"
  * this will fail, because that counter doesn't exist (because it's not the correctly translated string)
4. Attempt to create the counter "\\System(*)\FooWrite Bytes per second"
  * This will succeed, because we've found the correct translation of the given English string.