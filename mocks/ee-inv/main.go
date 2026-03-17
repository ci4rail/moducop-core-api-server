package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  ee-inv <eeprom-path>")
		os.Exit(2)
	}

	fmt.Println("{")
	fmt.Println("  \"vendor\": \"Ci4Rail\",")
	fmt.Println("  \"model\": \"S100-MLC01\",")
	fmt.Println("  \"variant\": 0,")
	fmt.Println("  \"majorVersion\": 1,")
	fmt.Println("  \"serial\": \"12345\"")
	fmt.Println("}")
}
