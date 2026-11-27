package process

import (
	"reflect"
	"testing"
)

// The parser tests use trimmed copies of real command output, so they
// exercise the formats DevDoctor actually meets rather than an idealised
// version of them.

func TestParseNetstat(t *testing.T) {
	out := "\r\n" +
		"Active Connections\r\n" +
		"\r\n" +
		"  Proto  Local Address          Foreign Address        State           PID\r\n" +
		"  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       2096\r\n" +
		"  TCP    [::]:3000              [::]:0                 LISTENING       14232\r\n" +
		"  TCP    127.0.0.1:8000         0.0.0.0:0              LISTENING       555\r\n" +
		"  TCP    127.0.0.1:5432         127.0.0.1:51234        ESTABLISHED     900\r\n" +
		"  TCP    127.0.0.1:6379         0.0.0.0:0              SYN_SENT        0\r\n" +
		"  UDP    0.0.0.0:5353           *:*                                    1234\r\n" +
		"  TCP    broken-address        0.0.0.0:0              LISTENING       1\r\n"

	got := parseNetstat(out)

	want := []Listener{
		{Port: 135, Protocol: "tcp", PID: 2096},
		{Port: 3000, Protocol: "tcp", PID: 14232},
		{Port: 8000, Protocol: "tcp", PID: 555},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNetstat() = %+v, want %+v", got, want)
	}
}

func TestParseNetstatKeepsPortWithoutPID(t *testing.T) {
	// A row whose PID column is unreadable still proves the port is
	// occupied, so it must survive with PID 0 rather than be dropped.
	got := parseNetstat("  TCP    0.0.0.0:8080         0.0.0.0:0              LISTENING       not-a-pid\r\n")
	want := []Listener{{Port: 8080, Protocol: "tcp"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNetstat() = %+v, want %+v", got, want)
	}
}

func TestParseTasklist(t *testing.T) {
	out := "\"System Idle Process\",\"0\",\"Services\",\"0\",\"8 K\"\r\n" +
		"\"System\",\"4\",\"Services\",\"0\",\"1,120 K\"\r\n" +
		"\"python.exe\",\"14232\",\"Console\",\"1\",\"35,216 K\"\r\n" +
		"INFO: No tasks are running which match the specified criteria.\r\n"

	got := parseTasklist(out)

	want := map[int]string{
		0:     "System Idle Process",
		4:     "System",
		14232: "python.exe",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTasklist() = %+v, want %+v", got, want)
	}
}

func TestParseTasklistEmpty(t *testing.T) {
	if got := parseTasklist(""); len(got) != 0 {
		t.Errorf("parseTasklist(\"\") = %+v, want an empty map", got)
	}
}

func TestParseProcNetTCP(t *testing.T) {
	// 1F90 = 8080, 0BB8 = 3000, 0016 = 22.
	out := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 11111 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 22222 1 0000000000000000 100 0 0 10 0\n" +
		"   2: 0100007F:0BB8 0100007F:C350 01 00000000:00000000 00:00000000 00000000  1000        0 33333 1 0000000000000000 20 4 30 10 -1\n"

	got := parseProcNetTCP(out)

	want := []Listener{
		{Port: 22, Protocol: "tcp", inode: "11111"},
		{Port: 8080, Protocol: "tcp", inode: "22222"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseProcNetTCP() = %+v, want %+v", got, want)
	}
}

func TestSocketInode(t *testing.T) {
	cases := []struct {
		target string
		want   string
		ok     bool
	}{
		{"socket:[123456]", "123456", true},
		{"/dev/null", "", false},
		{"anon_inode:[eventpoll]", "", false},
		{"socket:[", "", false},
	}
	for _, tc := range cases {
		got, ok := socketInode(tc.target)
		if ok != tc.ok || got != tc.want {
			t.Errorf("socketInode(%q) = (%q, %v), want (%q, %v)", tc.target, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSnapshotFromFiltersSortsAndDeduplicates(t *testing.T) {
	all := []Listener{
		{Port: 22, Protocol: "tcp", PID: 1, Process: "sshd"},
		{Port: 3000, Protocol: "tcp", PID: 111, Process: "node"},
		{Port: 8000, Protocol: "tcp", PID: 222, Process: ""},           // IPv6 row, name not read
		{Port: 8000, Protocol: "tcp", PID: 222, Process: "python.exe"}, // IPv4 row, named
		{Port: 6379, Protocol: "tcp", PID: 0, Process: ""},
	}

	got := snapshotFrom(all, []int{8000, 3000, 6379, 5432})

	// Sorted by port, one entry per port, named owner preferred.
	want := []Listener{
		{Port: 3000, Protocol: "tcp", PID: 111, Process: "node"},
		{Port: 6379, Protocol: "tcp"},
		{Port: 8000, Protocol: "tcp", PID: 222, Process: "python.exe"},
	}
	if !reflect.DeepEqual(got.Ports, want) {
		t.Errorf("snapshotFrom() = %+v, want %+v", got.Ports, want)
	}
	if got.Note != "" {
		t.Errorf("Note = %q, want empty for a successful inspection", got.Note)
	}
}

func TestSnapshotFromEmptyIsNotNil(t *testing.T) {
	// An empty result must encode as [] in JSON, not null.
	got := snapshotFrom(nil, DevPorts)
	if got.Ports == nil {
		t.Error("Ports should be an empty slice, not nil")
	}
	if len(got.Ports) != 0 {
		t.Errorf("Ports = %+v, want none", got.Ports)
	}
}

func TestSnapshotOfIsSafeToCall(t *testing.T) {
	// Smoke test against the real machine: it must return without
	// panicking, and either report ports or explain why it cannot.
	// No assertion on content, because the ports in use vary.
	snap := SnapshotOf(DevPorts)
	for _, l := range snap.Ports {
		if l.Port <= 0 {
			t.Errorf("Listener has an invalid port: %+v", l)
		}
	}
	if snap.Ports == nil {
		t.Error("Ports should be an empty slice, not nil")
	}
}

func TestUnsupportedInspectionUsesAnEmptyPortsSlice(t *testing.T) {
	// The macOS/other-platform adapter returns ErrUnsupported. Exercise the
	// public result contract here so JSON consumers see [] instead of null on
	// platforms where inspection is unavailable.
	snap := unavailableSnapshot("unavailable")
	if snap.Ports == nil {
		t.Fatal("Ports must be an empty slice when inspection is unavailable")
	}
}

func TestDevPortsAreValid(t *testing.T) {
	seen := map[int]bool{}
	for _, port := range DevPorts {
		if port <= 0 || port > 65535 {
			t.Errorf("DevPorts contains an invalid port: %d", port)
		}
		if seen[port] {
			t.Errorf("DevPorts contains %d twice", port)
		}
		seen[port] = true
	}
}
