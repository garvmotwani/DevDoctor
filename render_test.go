package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"devdoctor/internal/diagnostics"
	"devdoctor/internal/project"
	"devdoctor/internal/system"
	"devdoctor/internal/tools"
)

func sampleResult() result {
	return result{
		System: system.Info{OS: "windows", Architecture: "amd64"},
		Tools: []tools.Result{
			{
				Tool:    tools.Tool{Name: "Git", Command: "git"},
				Status:  tools.StatusOK,
				Version: "2.51.0",
				Path:    `C:\Users\secret-user\bin\git.exe`,
			},
			{
				Tool:   tools.Tool{Name: "Docker", Command: "docker"},
				Status: tools.StatusNotFound,
			},
			{
				Tool:   tools.Tool{Name: "Python", Command: "python"},
				Status: tools.StatusError,
				Path:   `C:\Users\secret-user\python.exe`,
				Err:    errors.New("boom: internal detail"),
			},
		},
		Report: project.Report{
			Root:  `C:\work\some-project`,
			Facts: []project.Fact{{Label: "Node.js project"}, {Label: "package manager", Detail: "npm"}},
		},
		Diagnostics: []diagnostics.Diagnostic{
			{
				ID:          "PY-001",
				Severity:    diagnostics.SeverityInfo,
				Title:       "No Python environment detected",
				Description: "noted",
				Evidence:    "requirements.txt exists",
			},
			{
				ID:           "DEP-001",
				Severity:     diagnostics.SeverityWarning,
				Title:        "Node dependencies may not be installed",
				Description:  "node_modules is missing",
				Evidence:     "package.json exists; node_modules does not exist.",
				SuggestedFix: "npm install",
			},
			{
				ID:          "GIT-001",
				Severity:    diagnostics.SeverityError,
				Title:       "Not a repository",
				Description: "no .git",
				Evidence:    ".git does not exist",
			},
		},
	}
}

func TestRenderJSONShape(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJSON(&buf, sampleResult()); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON:\n%s", buf.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, key := range []string{"version", "root", "system", "tools", "facts", "diagnostics", "summary"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("JSON is missing top-level key %q", key)
		}
	}
	if got := doc["version"]; got != Version {
		t.Errorf("version = %v, want %v", got, Version)
	}
	if got := doc["root"]; got != `C:\work\some-project` {
		t.Errorf("root = %v, want the scanned directory", got)
	}

	sys, ok := doc["system"].(map[string]any)
	if !ok || sys["os"] != "windows" || sys["architecture"] != "amd64" {
		t.Errorf("system = %v, want os/architecture", doc["system"])
	}

	toolList, ok := doc["tools"].([]any)
	if !ok || len(toolList) != 3 {
		t.Fatalf("tools = %v, want 3 entries", doc["tools"])
	}
	git := toolList[0].(map[string]any)
	if git["name"] != "Git" || git["status"] != "ok" || git["version"] != "2.51.0" {
		t.Errorf("git tool = %v, want name/status/version", git)
	}
	docker := toolList[1].(map[string]any)
	if docker["status"] != "not_found" {
		t.Errorf("docker status = %v, want not_found", docker["status"])
	}
	if _, ok := docker["version"]; ok {
		t.Errorf("docker should omit version when unknown, got %v", docker["version"])
	}
	python := toolList[2].(map[string]any)
	if python["status"] != "error" {
		t.Errorf("python status = %v, want error", python["status"])
	}

	facts, ok := doc["facts"].([]any)
	if !ok || len(facts) != 2 {
		t.Fatalf("facts = %v, want 2 entries", doc["facts"])
	}

	diags, ok := doc["diagnostics"].([]any)
	if !ok || len(diags) != 3 {
		t.Fatalf("diagnostics = %v, want 3 entries", doc["diagnostics"])
	}
	dep := diags[1].(map[string]any)
	for key, want := range map[string]string{
		"id":           "DEP-001",
		"severity":     "warning",
		"title":        "Node dependencies may not be installed",
		"description":  "node_modules is missing",
		"evidence":     "package.json exists; node_modules does not exist.",
		"suggestedFix": "npm install",
	} {
		if dep[key] != want {
			t.Errorf("diagnostic %q = %v, want %q", key, dep[key], want)
		}
	}

	summary, ok := doc["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary = %v, want an object", doc["summary"])
	}
	if summary["info"] != 1.0 || summary["warnings"] != 1.0 || summary["errors"] != 1.0 {
		t.Errorf("summary = %v, want info 1, warnings 1, errors 1", summary)
	}
}

func TestRenderJSONOmitsToolPathsAndErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJSON(&buf, sampleResult()); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	out := buf.String()
	for _, forbidden := range []string{"secret-user", "git.exe", "python.exe", "boom", `"Path"`, `"Err"`} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON must not contain %q:\n%s", forbidden, out)
		}
	}
}

func TestRenderJSONDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	if err := renderJSON(&first, sampleResult()); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	if err := renderJSON(&second, sampleResult()); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("two renders differ:\n--- first ---\n%s\n--- second ---\n%s", first.String(), second.String())
	}
}

func TestRenderJSONEmptySlices(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJSON(&buf, result{}); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"tools": []`, `"facts": []`, `"diagnostics": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("empty result should encode %s, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "null") {
		t.Errorf("empty result must not contain null, got:\n%s", out)
	}
}

// TestRenderJSONNeverLeaksEnvValues is the machine-readable half of the
// secret-safety guarantee: .env values cannot appear because the parser
// never stores them, and this test proves the JSON document reflects
// that for a realistic file.
func TestRenderJSONNeverLeaksEnvValues(t *testing.T) {
	stubTools(t)
	const secret = "sk-live-DO-NOT-PRINT-12345"

	dir := writeFixture(t, map[string]string{
		".env":         "API_KEY=" + secret + "\nDATABASE_URL=\nnot a var line\n",
		".env.example": "API_KEY=\nDATABASE_URL=\nPORT=\n",
	})

	res, err := collect(dir)
	if err != nil {
		t.Fatalf("collect(%q) error: %v", dir, err)
	}

	var buf bytes.Buffer
	if err := renderJSON(&buf, res); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("output is not valid JSON:\n%s", buf.String())
	}

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("JSON leaked a .env value:\n%s", out)
	}
	// The names themselves are safe and useful, so they must survive.
	for _, want := range []string{"ENV-002", "ENV-003", "ENV-004", "PORT", "DATABASE_URL"} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON should mention %q:\n%s", want, out)
		}
	}
}

func TestRenderHumanKeepsTextLayout(t *testing.T) {
	var buf bytes.Buffer
	renderHuman(&buf, sampleResult())
	out := buf.String()

	for _, want := range []string{
		"DevDoctor v" + Version,
		"SYSTEM",
		"OS: windows",
		"Architecture: amd64",
		"DEVELOPER TOOLS",
		"✓ Git      2.51.0",
		"✗ Docker   Not found",
		"✗ Python   Found but could not get version",
		"PROJECT",
		"Scanning: " + `C:\work\some-project`,
		"✓ package manager — npm",
		"DIAGNOSIS",
		"[1] ℹ No Python environment detected",
		"Evidence: package.json exists; node_modules does not exist.",
		"        npm install",
		"2 problems found.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	// Tool paths are machine detail and never appear in the text report.
	if strings.Contains(out, "secret-user") {
		t.Errorf("human output must not contain tool paths:\n%s", out)
	}
}

func TestRenderHumanNoProblems(t *testing.T) {
	res := result{Diagnostics: []diagnostics.Diagnostic{
		{ID: "PY-001", Severity: diagnostics.SeverityInfo, Title: "note"},
	}}
	var buf bytes.Buffer
	renderHuman(&buf, res)
	if !strings.Contains(buf.String(), "No problems found.") {
		t.Errorf("info-only run should say no problems:\n%s", buf.String())
	}
}
