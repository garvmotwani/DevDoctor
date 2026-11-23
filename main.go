package main

import (
	"fmt"
	"os"
	"time"

	"devdoctor/internal/diagnostics"
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

	// DIAGNOSIS
	//
	// Rules are pure functions over the evidence we already collected;
	// nothing here touches the disk again.
	diags := diagnostics.Run(rep, diagnostics.DefaultRules())

	fmt.Println()
	fmt.Println("DIAGNOSIS")
	fmt.Println("────────────────────────────")
	fmt.Println()

	problems := 0
	for i, d := range diags {
		fmt.Printf("[%d] %s %s\n", i+1, severitySymbol(d.Severity), d.Title)
		fmt.Printf("    %s (%s)\n", d.ID, d.Severity)
		fmt.Println()
		fmt.Printf("    %s\n", d.Description)
		fmt.Println()
		fmt.Printf("    Evidence: %s\n", d.Evidence)
		if d.SuggestedFix != "" {
			fmt.Println()
			fmt.Println("    Suggested fix:")
			fmt.Println()
			fmt.Printf("        %s\n", d.SuggestedFix)
		}
		fmt.Println()

		// Informational notes are shown above but do not count as
		// problems; only warnings and errors do.
		if d.Severity >= diagnostics.SeverityWarning {
			problems++
		}
	}

	if problems == 0 {
		fmt.Println("No problems found.")
	} else {
		fmt.Printf("%s found.\n", plural(problems, "problem"))
	}
}

// severitySymbol maps a Severity to the CLI glyph used for it.
func severitySymbol(s diagnostics.Severity) string {
	switch s {
	case diagnostics.SeverityError:
		return "✗"
	case diagnostics.SeverityWarning:
		return "⚠"
	default:
		return "ℹ"
	}
}

// plural formats counts: 1 "problem", 2 "problems".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
