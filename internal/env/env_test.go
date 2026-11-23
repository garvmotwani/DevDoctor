package env

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		wantKeys      []string
		wantEmpty     []string
		wantMalformed []int
	}{
		{
			name:     "simple pairs",
			content:  "DATABASE_URL=postgres://localhost/app\nPORT=8000\n",
			wantKeys: []string{"DATABASE_URL", "PORT"},
		},
		{
			name:     "comments, blank lines and export",
			content:  "# a comment\n\n  export API_KEY=abc  \n",
			wantKeys: []string{"API_KEY"},
		},
		{
			name:      "empty values in three spellings",
			content:   "EMPTY=\nQUOTED=\"\"\nSPACED=   \n",
			wantKeys:  []string{"EMPTY", "QUOTED", "SPACED"},
			wantEmpty: []string{"EMPTY", "QUOTED", "SPACED"},
		},
		{
			name:          "malformed lines are counted, not guessed",
			content:       "GOOD=1\nNO_EQUALS\n=VALUE\nBAD KEY=1\n9STARTS_WITH_DIGIT=1\n",
			wantKeys:      []string{"GOOD"},
			wantMalformed: []int{2, 3, 4, 5},
		},
		{
			name:     "quoted whitespace is a real value",
			content:  "TOKEN=\"  \"\n",
			wantKeys: []string{"TOKEN"},
		},
		{
			name:     "duplicate key, last value wins",
			content:  "MODE=\nMODE=dev\n",
			wantKeys: []string{"MODE"},
		},
		{
			name:      "duplicate key, last empty wins",
			content:   "MODE=dev\nMODE=\n",
			wantKeys:  []string{"MODE"},
			wantEmpty: []string{"MODE"},
		},
		{
			name:     "windows line endings",
			content:  "A=1\r\nB=2\r\n",
			wantKeys: []string{"A", "B"},
		},
		{
			name:     "empty content",
			content:  "",
			wantKeys: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse([]byte(tc.content))
			if !slices.Equal(got.Keys, tc.wantKeys) {
				t.Errorf("Keys = %v, want %v", got.Keys, tc.wantKeys)
			}
			if !slices.Equal(got.Empty, tc.wantEmpty) {
				t.Errorf("Empty = %v, want %v", got.Empty, tc.wantEmpty)
			}
			if !slices.Equal(got.Malformed, tc.wantMalformed) {
				t.Errorf("Malformed = %v, want %v", got.Malformed, tc.wantMalformed)
			}
		})
	}
}

// TestParseKeepsNoValues is the security guarantee in executable form:
// after parsing, the value text must not exist anywhere in the result.
func TestParseKeepsNoValues(t *testing.T) {
	const secret = "hunter2-super-secret"
	f := Parse([]byte("DATABASE_PASSWORD=" + secret + "\nPORT=5432\n"))

	dump := fmt.Sprintf("%+v", f)
	for _, value := range []string{secret, "5432"} {
		if strings.Contains(dump, value) {
			t.Fatalf("parsed File still contains value %q: %s", value, dump)
		}
	}
	if !f.Has("DATABASE_PASSWORD") || !f.Has("PORT") {
		t.Fatalf("expected both names to be recorded: %+v", f)
	}
}

func TestHasOnNilFile(t *testing.T) {
	var f *File
	if f.Has("ANYTHING") {
		t.Fatal("nil File must declare nothing")
	}
}

func TestParseFS(t *testing.T) {
	fsys := fstest.MapFS{
		"app/.env": &fstest.MapFile{Data: []byte("A=1\nB=\n")},
	}
	f, err := ParseFS(fsys, "app/.env")
	if err != nil {
		t.Fatalf("ParseFS: %v", err)
	}
	if !slices.Equal(f.Keys, []string{"A", "B"}) {
		t.Errorf("Keys = %v", f.Keys)
	}
	if !slices.Equal(f.Empty, []string{"B"}) {
		t.Errorf("Empty = %v", f.Empty)
	}
}
