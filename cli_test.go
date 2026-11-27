package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devdoctor/internal/tools"
)

// stubTools keeps tests hermetic: no version commands are executed.
func stubTools(t *testing.T) {
	t.Helper()
	original := checkTools
	checkTools = func() []tools.Result { return nil }
	t.Cleanup(func() { checkTools = original })
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{name: "no arguments scans here", args: nil, want: options{}},
		{name: "json only", args: []string{"--json"}, want: options{json: true}},
		{name: "single dash json", args: []string{"-json"}, want: options{json: true}},
		{name: "scan alone", args: []string{"scan"}, want: options{}},
		{name: "scan with directory", args: []string{"scan", "./some-project"}, want: options{dir: "./some-project"}},
		{name: "scan with flag before dir", args: []string{"scan", "--json", "./p"}, want: options{json: true, dir: "./p"}},
		{name: "flag before subcommand", args: []string{"--json", "scan", "./p"}, want: options{json: true, dir: "./p"}},
		{name: "help long", args: []string{"--help"}, want: options{help: true}},
		{name: "help short", args: []string{"-h"}, want: options{help: true}},
		{name: "help subcommand", args: []string{"help"}, want: options{help: true}},
		{name: "unknown command", args: []string{"build"}, wantErr: true},
		{name: "unknown command with json", args: []string{"--json", "doctor"}, wantErr: true},
		{name: "too many arguments", args: []string{"scan", "a", "b"}, wantErr: true},
		{name: "unknown flag", args: []string{"--verbose"}, wantErr: true},
		{name: "bare dash", args: []string{"-"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseArgs(%q) = %+v, want error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%q) returned unexpected error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestResolveRoot(t *testing.T) {
	t.Run("empty means current directory", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		got, err := resolveRoot("")
		if err != nil {
			t.Fatalf("resolveRoot(\"\") error: %v", err)
		}
		if got != wd {
			t.Errorf("resolveRoot(\"\") = %q, want %q", got, wd)
		}
	})

	t.Run("relative path becomes absolute", func(t *testing.T) {
		dir := t.TempDir()
		got, err := resolveRoot(dir)
		if err != nil {
			t.Fatalf("resolveRoot(%q) error: %v", dir, err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("resolveRoot(%q) = %q, want an absolute path", dir, got)
		}
		if filepath.Clean(got) != filepath.Clean(dir) {
			t.Errorf("resolveRoot(%q) = %q, want the same directory", dir, got)
		}
	})

	t.Run("missing path is an error", func(t *testing.T) {
		if _, err := resolveRoot(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
			t.Error("resolveRoot on a missing path returned no error")
		}
	})

	t.Run("file is not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := resolveRoot(file); err == nil {
			t.Error("resolveRoot on a file returned no error")
		}
	})
}

// writeFixture creates a throwaway project directory from name -> content.
// A content of "" creates an empty file; a trailing "/" creates a directory.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatalf("MkdirAll %s: %v", path, err)
			}
			continue
		}
		if parent := filepath.Dir(path); parent != dir {
			if err := os.MkdirAll(parent, 0o700); err != nil {
				t.Fatalf("MkdirAll %s: %v", parent, err)
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
	return dir
}

func TestRunExitCodes(t *testing.T) {
	stubTools(t)

	clean := writeFixture(t, map[string]string{
		".git/":            "",
		"package.json":     `{"dependencies":{}}`,
		"node_modules/":    "",
		".env":             "PORT=3000\n",
		".env.example":     "PORT=\n",
		"requirements.txt": "",
		".venv/":           "",
	})
	warning := writeFixture(t, map[string]string{
		"package.json": `{"dependencies":{"express":"4"}}`,
	})
	infoOnly := writeFixture(t, map[string]string{
		"requirements.txt": "flask\n",
	})
	filePath := filepath.Join(infoOnly, "requirements.txt")

	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string // substring the error output must contain
		wantUsage  bool   // usage errors also print the usage text
	}{
		{name: "clean project exits 0", args: []string{"scan", clean}, wantCode: exitOK},
		{name: "information only exits 0", args: []string{"scan", infoOnly}, wantCode: exitOK},
		{name: "warning exits 1", args: []string{"scan", warning}, wantCode: exitProblems},
		{name: "warning in json exits 1", args: []string{"scan", "--json", warning}, wantCode: exitProblems},
		{name: "unknown command exits 2", args: []string{"do-things"}, wantCode: exitFailure, wantStderr: "unknown command", wantUsage: true},
		{name: "too many arguments exits 2", args: []string{"scan", "a", "b"}, wantCode: exitFailure, wantStderr: "too many arguments", wantUsage: true},
		{name: "unknown flag exits 2", args: []string{"--nope"}, wantCode: exitFailure, wantStderr: "unknown flag", wantUsage: true},
		{name: "missing path exits 2", args: []string{"scan", filepath.Join(clean, "nope")}, wantCode: exitFailure, wantStderr: "does not exist"},
		{name: "file path exits 2", args: []string{"scan", filePath}, wantCode: exitFailure, wantStderr: "not a directory"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("run(%q) exit code = %d, want %d (stderr: %s)", tc.args, code, tc.wantCode, stderr.String())
			}
			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
			if strings.Contains(stderr.String(), "Usage:") != tc.wantUsage {
				t.Errorf("stderr usage text present = %v, want %v (stderr: %q)",
					strings.Contains(stderr.String(), "Usage:"), tc.wantUsage, stderr.String())
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != exitOK {
		t.Errorf("--help exit code = %d, want %d", code, exitOK)
	}
	for _, want := range []string{"Usage:", "devdoctor scan", "--json", "Exit codes:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunSelectsDirectory(t *testing.T) {
	stubTools(t)
	dir := writeFixture(t, map[string]string{"requirements.txt": "flask\n"})

	var stdout, stderr bytes.Buffer
	if code := run([]string{"scan", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Scanning: "+dir) {
		t.Errorf("output should name the scanned directory %q:\n%s", dir, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Python project") {
		t.Errorf("output should describe the scanned project:\n%s", stdout.String())
	}
}
