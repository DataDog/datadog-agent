package main

func init() {
	globalConfig = NewConfig()

	buildable := GetBuildableConfig()
	setupConfig(buildable)
}

func setupConfig(bcfg BuildableConfig) {
	bcfg.DefineSetting("apple", "green")
	bcfg.DefineSetting("banana", "yellow")
	bcfg.DefineSetting("cherry", "red")

	bcfg.DefineSetting("carrot", "orange")
	bcfg.DefineSetting("cucumber", "green")
	bcfg.DefineSetting("brocolli", "green")

	bcfg.DefineSetting("setting0", "zero")
	bcfg.DefineSetting("setting1", "one")
	bcfg.DefineSetting("setting2", "two")
	bcfg.DefineSetting("setting3", "three")
	bcfg.DefineSetting("setting4", "four")
	bcfg.DefineSetting("setting5", "five")
	bcfg.DefineSetting("setting6", "six")
	bcfg.DefineSetting("setting7", "seven")
	bcfg.DefineSetting("setting8", "eight")
	bcfg.DefineSetting("setting9", "nine")

	bcfg.DefineSetting("cost", 84)
	bcfg.DefineSetting("vegetables.enabled", true)

	bcfg.DefineSetting("chair", "wooden")
	bcfg.DefineSetting("car", "sedan")
}
