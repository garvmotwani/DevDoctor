package main

import (
	"encoding/json"
	"fmt"
	"io"

	"devdoctor/internal/diagnostics"
	"devdoctor/internal/tools"
)

// renderHuman writes the default text report. The layout is unchanged
// from earlier phases on purpose: this is still the output a person
// reads when they just type `devdoctor`.
func renderHuman(w io.Writer, res result) {
	fmt.Fprintf(w, "DevDoctor v%s\n", Version)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYSTEM")
	fmt.Fprintln(w, "────────────────")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "OS: %s\n", res.System.OS)
	fmt.Fprintf(w, "Architecture: %s\n", res.System.Architecture)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "DEVELOPER TOOLS")
	fmt.Fprintln(w, "────────────────────────────")
	fmt.Fprintln(w)
	for _, r := range res.Tools {
		switch r.Status {
		case tools.StatusOK:
			fmt.Fprintf(w, "✓ %-8s %s\n", r.Tool.Name, r.Version)
		case tools.StatusNotFound:
			fmt.Fprintf(w, "✗ %-8s Not found\n", r.Tool.Name)
		case tools.StatusError:
			fmt.Fprintf(w, "✗ %-8s Found but could not get version\n", r.Tool.Name)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "PROJECT")
	fmt.Fprintln(w, "────────────────────────────")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Scanning: %s\n", res.Report.Root)
	fmt.Fprintln(w)
	for _, f := range res.Report.Facts {
		if f.Detail != "" {
			fmt.Fprintf(w, "✓ %s — %s\n", f.Label, f.Detail)
		} else {
			fmt.Fprintf(w, "✓ %s\n", f.Label)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "DIAGNOSIS")
	fmt.Fprintln(w, "────────────────────────────")
	fmt.Fprintln(w)
	for i, d := range res.Diagnostics {
		fmt.Fprintf(w, "[%d] %s %s\n", i+1, severitySymbol(d.Severity), d.Title)
		fmt.Fprintf(w, "    %s (%s)\n", d.ID, d.Severity)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "    %s\n", d.Description)
		fmt.Fprintln(w)
		fmt.Fprintf(w, "    Evidence: %s\n", d.Evidence)
		if d.SuggestedFix != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "    Suggested fix:")
			fmt.Fprintln(w)
			fmt.Fprintf(w, "        %s\n", d.SuggestedFix)
		}
		fmt.Fprintln(w)
	}

	summary := summarize(res.Diagnostics)
	if problems := summary.Warnings + summary.Errors; problems == 0 {
		fmt.Fprintln(w, "No problems found.")
	} else {
		fmt.Fprintf(w, "%s found.\n", plural(problems, "problem"))
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

// jsonReport is the machine-readable document.
//
// Fields are declared in the order they appear in the output, and
// every slice is built with make so an empty result encodes as [] and
// not null. Tool paths and tool errors are intentionally absent: they
// describe the local machine, not the project, and are not something a
// consumer needs or a report should leak.
type jsonReport struct {
	Version     string           `json:"version"`
	Root        string           `json:"root"`
	System      jsonSystem       `json:"system"`
	Tools       []jsonTool       `json:"tools"`
	Facts       []jsonFact       `json:"facts"`
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
	Summary     jsonSummary      `json:"summary"`
}

type jsonSystem struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type jsonTool struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok", "not_found" or "error"
	Version string `json:"version,omitempty"`
}

type jsonFact struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type jsonDiagnostic struct {
	ID           string `json:"id"`
	Severity     string `json:"severity"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Evidence     string `json:"evidence"`
	SuggestedFix string `json:"suggestedFix"`
}

type jsonSummary struct {
	Info     int `json:"info"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

// renderJSON writes one JSON document describing the same run the
// text renderer would print.
func renderJSON(w io.Writer, res result) error {
	out := jsonReport{
		Version:     Version,
		Root:        res.Report.Root,
		System:      jsonSystem{OS: res.System.OS, Architecture: res.System.Architecture},
		Tools:       make([]jsonTool, 0, len(res.Tools)),
		Facts:       make([]jsonFact, 0, len(res.Report.Facts)),
		Diagnostics: make([]jsonDiagnostic, 0, len(res.Diagnostics)),
	}

	for _, r := range res.Tools {
		tool := jsonTool{Name: r.Tool.Name, Status: toolStatus(r.Status)}
		if r.Status == tools.StatusOK {
			tool.Version = r.Version
		}
		out.Tools = append(out.Tools, tool)
	}

	for _, f := range res.Report.Facts {
		out.Facts = append(out.Facts, jsonFact{Label: f.Label, Detail: f.Detail})
	}

	for _, d := range res.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, jsonDiagnostic{
			ID:           d.ID,
			Severity:     d.Severity.String(),
			Title:        d.Title,
			Description:  d.Description,
			Evidence:     d.Evidence,
			SuggestedFix: d.SuggestedFix,
		})
	}

	out.Summary = summarize(res.Diagnostics)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// No HTML escaping: this output targets machines, and escaping
	// stray < or & in user-derived text only makes it harder to read.
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// toolStatus maps a tool Status to its stable JSON value.
func toolStatus(s tools.Status) string {
	switch s {
	case tools.StatusOK:
		return "ok"
	case tools.StatusNotFound:
		return "not_found"
	default:
		return "error"
	}
}

// summarize counts diagnostics by severity. The text summary line, the
// JSON counts and the exit code are all derived from this one
// function, so they cannot drift apart.
func summarize(diags []diagnostics.Diagnostic) jsonSummary {
	var s jsonSummary
	for _, d := range diags {
		switch d.Severity {
		case diagnostics.SeverityError:
			s.Errors++
		case diagnostics.SeverityWarning:
			s.Warnings++
		default:
			s.Info++
		}
	}
	return s
}

// exitCodeFor maps the summary onto the documented exit codes.
// Informational notes alone are not problems, so they still exit 0.
func exitCodeFor(diags []diagnostics.Diagnostic) int {
	s := summarize(diags)
	if s.Warnings+s.Errors > 0 {
		return exitProblems
	}
	return exitOK
}
