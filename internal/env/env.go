// Package env reads .env files without ever keeping secret values.
//
// The security rule for DevDoctor is "never print a secret". The
// strongest way to honour it is structural: this package parses a
// .env file and reports only
//
//   - which variable names were declared,
//   - which were declared with an empty value,
//   - which line numbers could not be parsed.
//
// Values are read, inspected for emptiness, and thrown away. There is
// no field to leak because no field stores them.
package env

import (
	"io/fs"
	"strings"
)

// File is what we learned about one environment file. An empty
// File means "declared nothing", which is different from a missing
// file — callers use a nil *File for "absent" and a non-nil one with
// zero Keys for "present but empty".
type File struct {
	Keys      []string // variable names, first-seen order, deduplicated
	Empty     []string // names declared with no value (e.g. FOO= or FOO="")
	Malformed []int    // 1-based line numbers that are not NAME=value
}

// Has reports whether name is declared in the file, with or without a
// value. A nil *File declares nothing, so this is safe on nil.
func (f *File) Has(name string) bool {
	if f == nil {
		return false
	}
	for _, k := range f.Keys {
		if k == name {
			return true
		}
	}
	return false
}

// Parse reads .env content and returns only names and shapes.
//
// Recognised syntax is deliberately small:
//
//	KEY=value
//	export KEY=value
//	KEY="quoted value"
//	# comment
//
// Blank lines and comments are skipped. Anything else that is not
// NAME=value is recorded as a malformed line number. We do not try to
// be a full shell parser: guessing wrong is worse than saying "I could
// not parse this".
func Parse(content []byte) File {
	var f File
	empty := map[string]bool{} // name -> whether its latest value was empty
	seen := map[string]bool{}

	for i, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// "export FOO=1" is common in shell-sourced files. TrimPrefix
		// is harmless when the prefix is absent.
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		eq := strings.IndexByte(line, '=')
		if eq <= 0 { // no '=' at all, or '=' with no name before it
			f.Malformed = append(f.Malformed, i+1)
			continue
		}
		name := strings.TrimSpace(line[:eq])
		if !validName(name) {
			f.Malformed = append(f.Malformed, i+1)
			continue
		}

		value := unquote(strings.TrimSpace(line[eq+1:]))
		if !seen[name] {
			seen[name] = true
			f.Keys = append(f.Keys, name)
		}
		// Later declarations win, matching what dotenv loaders do.
		empty[name] = value == ""
	}

	// Derive Empty in key order so output is stable, not map order.
	for _, name := range f.Keys {
		if empty[name] {
			f.Empty = append(f.Empty, name)
		}
	}
	return f
}

// ParseFS reads and parses a file from any fs.FS, so tests can use
// fstest.MapFS instead of a real disk.
func ParseFS(fsys fs.FS, name string) (File, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return File{}, err
	}
	return Parse(data), nil
}

// validName reports whether name looks like a POSIX environment
// variable name: letters, digits and underscores, not starting with a
// digit. "BAD KEY=1" and "=VALUE" are malformed rather than guesses.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
			// always fine
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// unquote strips one matching pair of surrounding quotes. Values with
// no quotes, or mismatched quotes, are returned as-is.
func unquote(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
