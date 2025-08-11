package main

import "fmt"

func FruitCollect() {
	cfg := GetConfig()
	val := cfg.Get("apple")
	fmt.Printf("apple => %s\n", val)
	val = cfg.Get("banana")
	fmt.Printf("banana => %s\n", val)
	val = cfg.Get("cherry")
	fmt.Printf("cherry => %s\n", val)
}

func FurnitureCollect() {
	cfg := GetConfig()
	val := cfg.GetString("chair")
	fmt.Printf("chair => %s\n", val)
}

func VehicleCollect() {
	cfg := GetConfig()
	val := cfg.GetString("car")
	fmt.Printf("car => %s\n", val)
}

func SettingsCollect() {
	cfg := GetConfig()
	for k := range 10 {
		fmt.Printf("setting%d => %s\n", k, cfg.Get(fmt.Sprintf("setting%d", k)))
	}
}

func VegetablesCollect() {
	cfg := GetConfig()
	fmt.Printf("cost => %d\n", cfg.GetInt("cost"))
	if cfg.GetBool("vegetables.enabled") {
		fmt.Printf("carrot => %s\n", cfg.Get("carrot"))
		fmt.Printf("cucumber => %s\n", cfg.Get("cucumber"))
		fmt.Printf("brocolli => %s\n", cfg.Get("brocolli"))
	}
}

//////////////////////

func FruitCollect2() {
	cfg := GetConfig()
	val := cfg.GetString("apple")
	fmt.Printf("apple => %s\n", val)
	val = cfg.GetString("banana")
	fmt.Printf("banana => %s\n", val)
	val = cfg.GetString("cherry")
	fmt.Printf("cherry => %s\n", val)
}

func FurnitureCollect2() {
	cfg := GetConfig()
	val := cfg.Get("chair")
	fmt.Printf("chair => %s\n", val)
}

func VehicleCollect2() {
	cfg := GetConfig()
	val := cfg.Get("car")
	fmt.Printf("car => %s\n", val)
}

func SettingsCollect2() {
	cfg := GetConfig()
	for k := range 10 {
		fmt.Printf("setting%d => %s\n", k, cfg.GetString(fmt.Sprintf("setting%d", k)))
	}
}

func VegetablesCollect2() {
	cfg := GetConfig()
	fmt.Printf("cost => %d\n", cfg.GetInt("cost"))
	if cfg.GetBool("vegetables.enabled") {
		fmt.Printf("carrot => %s\n", cfg.GetString("carrot"))
		fmt.Printf("cucumber => %s\n", cfg.GetString("cucumber"))
		fmt.Printf("brocolli => %s\n", cfg.GetString("brocolli"))
	}
}

//////////////////////

func FruitCollect3() {
	cfg := GetConfig()
	val := cfg.Get("apple")
	fmt.Printf("apple : %s\n", val)
	val = cfg.Get("banana")
	fmt.Printf("banana : %s\n", val)
	val = cfg.Get("cherry")
	fmt.Printf("cherry : %s\n", val)
}

func FurnitureCollect3() {
	cfg := GetConfig()
	val := cfg.GetString("chair")
	fmt.Printf("chair : %s\n", val)
}

func VehicleCollect3() {
	cfg := GetConfig()
	val := cfg.GetString("car")
	fmt.Printf("car : %s\n", val)
}

func SettingsCollect3() {
	cfg := GetConfig()
	for k := range 10 {
		fmt.Printf("setting%d : %s\n", k, cfg.Get(fmt.Sprintf("setting%d", k)))
	}
}

func VegetablesCollect3() {
	cfg := GetConfig()
	fmt.Printf("cost => %d\n", cfg.GetInt("cost"))
	if cfg.GetBool("vegetables.enabled") {
		fmt.Printf("carrot : %s\n", cfg.Get("carrot"))
		fmt.Printf("cucumber : %s\n", cfg.Get("cucumber"))
		fmt.Printf("brocolli : %s\n", cfg.Get("brocolli"))
	}
}

//////////////////////

func FruitCollect4() {
	cfg := GetConfig()
	val := cfg.GetString("apple")
	fmt.Printf("apple => %s\n", val)
	val = cfg.GetString("banana")
	fmt.Printf("banana => %s\n", val)
	val = cfg.GetString("cherry")
	fmt.Printf("cherry => %s\n", val)
}

func FurnitureCollect4() {
	cfg := GetConfig()
	val := cfg.Get("chair")
	fmt.Printf("chair => %s\n", val)
}

func VehicleCollect4() {
	cfg := GetConfig()
	val := cfg.Get("car")
	fmt.Printf("car => %s\n", val)
}

func SettingsCollect4() {
	cfg := GetConfig()
	for k := range 10 {
		fmt.Printf("setting%d => %s\n", k, cfg.GetString(fmt.Sprintf("setting%d", k)))
	}
}

func VegetablesCollect4() {
	cfg := GetConfig()
	fmt.Printf("cost => %d\n", cfg.GetInt("cost"))
	if cfg.GetBool("vegetables.enabled") {
		fmt.Printf("carrot => %s\n", cfg.GetString("carrot"))
		fmt.Printf("cucumber => %s\n", cfg.GetString("cucumber"))
		fmt.Printf("brocolli => %s\n", cfg.GetString("brocolli"))
	}
}
