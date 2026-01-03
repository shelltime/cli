package main

import (
	"fmt"
	"time"

	"github.com/malamtime/cli/stloader"
)

func main() {
	fmt.Println("Testing stloader package...")
	fmt.Println()

	// Test with shining effect
	l := stloader.NewLoader(stloader.LoaderConfig{
		Text:          "Processing your request, please wait...",
		EnableShining: true,
		BaseColor:     stloader.RGB{R: 100, G: 180, B: 255}, // Light blue
		ShineInterval: 100 * time.Millisecond,
		SpinInterval:  150 * time.Millisecond,
	})

	l.Start()
	time.Sleep(10 * time.Second)
	l.Stop()

	fmt.Println()
	fmt.Println("Done!")
}
