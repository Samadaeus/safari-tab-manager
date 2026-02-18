// main.go

package main

import (
	"flag"
	"fmt"
)

func main() {
	// Define a flag to support Safari Technology Preview
	preview := flag.Bool("preview", false, "Run with Safari Technology Preview")
	flag.Parse()

	if *preview {
		// Logic for Safari Technology Preview
		fmt.Println("Running Safari Technology Preview...")
	} else {
		// Logic for standard Safari
		fmt.Println("Running standard Safari...")
	}
}