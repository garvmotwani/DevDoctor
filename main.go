package main

import (
	"fmt"
	"os"
	"time"

	"devdoctor/internal/project"
	"devdoctor/internal/system"
	"devdoctor/internal/tools"
)

func main() {
	fmt.Println("DevDoctor v0.1")

	// SYSTEM
	info := system.GetInfo()
	fmt.Println()
	fmt.Println("SYSTEM")
	fmt.Println("────────────────")
	fmt.Println()
	fmt.Printf("OS: %s\n", info.OS)
	fmt.Printf("Architecture: %s\n", info.Architecture)

	// DEVELOPER TOOLS
	results := tools.CheckAll(tools.Defaults(), 3*time.Second)
	fmt.Println()
	fmt.Println("DEVELOPER TOOLS")
	fmt.Println("────────────────────────────")
	fmt.Println()
	for _, r := range results {
		switch r.Status {
		case tools.StatusOK:
			fmt.Printf("✓ %-8s %s\n", r.Tool.Name, r.Version)
		case tools.StatusNotFound:
			fmt.Printf("✗ %-8s Not found\n", r.Tool.Name)
		case tools.StatusError:
			fmt.Printf("✗ %-8s Found but could not get version\n", r.Tool.Name)
		}
	}

	// PROJECT
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("✗ could not determine working directory: %v\n", err)
		os.Exit(1)
	}

	rep, err := project.ScanPath(wd)
	if err != nil {
		fmt.Printf("✗ could not scan project: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("PROJECT")
	fmt.Println("────────────────────────────")
	fmt.Println()
	fmt.Printf("Scanning: %s\n", wd)
	fmt.Println()
	for _, f := range rep.Facts {
		if f.Detail != "" {
			fmt.Printf("✓ %s — %s\n", f.Label, f.Detail)
		} else {
			fmt.Printf("✓ %s\n", f.Label)
		}
	}
}
