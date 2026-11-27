// Package process answers one question: which development ports are
// occupied, and which process owns them?
//
// Operating systems expose this differently, so the package is split:
//
//	process.go          types, port list, filtering (platform neutral)
//	parsers.go          pure text parsers (testable on any OS)
//	inspect_windows.go  netstat + tasklist adapter
//	inspect_linux.go    /proc adapter
//	inspect_other.go    explicit "not implemented here" stub
//
// The shared layer is the contract: platform code returns whatever
// listeners it can see, and SnapshotOf filters them down to the ports
// DevDoctor cares about. That keeps the diagnostics identical on every
// OS even though the discovery mechanism is not.
//
// Two rules hold on every platform:
//
//   - DevDoctor never stops, suspends or signals a process. It only
//     reads and reports.
//   - When a platform or a permission level does not allow a fact to be
//     read, the fact is reported as unavailable. DevDoctor never
//     guesses a process name to fill the gap.
package process

import (
	"errors"
	"sort"
)

// DevPorts are the common development ports DevDoctor looks at, in the
// order a person usually thinks about them: web servers first, then
// databases.
var DevPorts = []int{3000, 5173, 8000, 8080, 5432, 6379}

// ErrUnsupported means this build runs on a platform whose inspection
// has not been implemented. It is a normal answer, not a failure: a
// scan still finishes, it just reports the limitation.
var ErrUnsupported = errors.New("process and port inspection is not implemented on this platform")

// Listener is one listening TCP socket.
type Listener struct {
	Port     int    // local port
	Protocol string // always "tcp" for now
	PID      int    // owning process, 0 when the OS did not tell us
	Process  string // process name, "" when unavailable

	// inode identifies the socket in /proc on Linux, which is how a
	// socket is matched to the process that holds it. Unused by the
	// other platforms, and never printed.
	inode string
}

// Snapshot is what one inspection produced.
//
// Note is "" when inspection worked. When it is set it explains, in
// user-facing words, why some or all process information is missing —
// and it deliberately never carries the underlying error: that is
// local machine detail, not something a report or a JSON consumer
// needs.
type Snapshot struct {
	Ports []Listener
	Note  string
}

// SnapshotOf inspects the machine and keeps only the wanted ports.
//
// An inspection failure is not returned as an error on purpose. Ports
// are a secondary signal: if they cannot be read, the project scan is
// still worth showing.
func SnapshotOf(wanted []int) Snapshot {
	all, err := inspect()
	switch {
	case errors.Is(err, ErrUnsupported):
		return Snapshot{Note: "Reading listening ports and their owning processes is not implemented for this platform yet."}
	case err != nil:
		return Snapshot{Note: "Listening ports could not be read on this system, possibly because of permissions."}
	}
	return snapshotFrom(all, wanted)
}

// snapshotFrom is the platform-neutral half of SnapshotOf: filter,
// de-duplicate and sort. It takes the raw listeners as input so it can
// be tested with fabricated data on any OS.
//
// Duplicates happen for real: a server listening on both IPv4 and IPv6
// reports the same port twice.
func snapshotFrom(all []Listener, wanted []int) Snapshot {
	want := make(map[int]bool, len(wanted))
	for _, port := range wanted {
		want[port] = true
	}

	byPort := make(map[int]Listener, len(wanted))
	for _, l := range all {
		if !want[l.Port] {
			continue
		}
		// Keep one entry per port, preferring the socket we could
		// attribute to a process over one we could not.
		if prev, seen := byPort[l.Port]; seen && (prev.Process != "" || l.Process == "") {
			continue
		}
		byPort[l.Port] = l
	}

	ports := make([]Listener, 0, len(byPort))
	for _, l := range byPort {
		ports = append(ports, l)
	}
	// Map iteration is random; sorting makes output deterministic.
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })

	return Snapshot{Ports: ports}
}
