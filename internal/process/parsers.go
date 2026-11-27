package process

import (
	"encoding/csv"
	"strconv"
	"strings"
)

// This file holds the pure parsers: text in, listeners out. No build
// tags, no disk, no child processes — so the Windows parser is tested
// on Linux and the Linux parser is tested on Windows.
//
// The samples used in the tests are trimmed copies of real output; that
// is the point of separating parsing from running the command.

// parseNetstat reads the output of `netstat -ano -p tcp` (Windows).
//
// A listening row looks like this, with the local address differing for
// IPv4 and IPv6:
//
//	Proto  Local Address          Foreign Address        State           PID
//	TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       2096
//	TCP    [::]:3000              [::]:0                 LISTENING       14232
//
// Only LISTENING TCP rows are interesting: ESTABLISHED rows are
// connections, not servers, and a client connection does not occupy a
// port in the way that blocks a dev server.
func parseNetstat(out string) []Listener {
	var listeners []Listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line) // also strips the trailing \r
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[0], "TCP") || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		port, ok := addrPort(fields[1])
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil || pid <= 0 {
			// Without a PID we cannot attribute the port, but the port
			// is still occupied; keep it with PID 0 and say so later.
			pid = 0
		}
		listeners = append(listeners, Listener{Port: port, Protocol: "tcp", PID: pid})
	}
	return listeners
}

// addrPort extracts the port from "0.0.0.0:135", "127.0.0.1:22" or
// "[::]:3000". Splitting at the last colon handles all three.
func addrPort(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 || i == len(addr)-1 {
		return 0, false
	}
	port, err := strconv.Atoi(addr[i+1:])
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

// parseTasklist reads `tasklist /FO CSV /NH` (Windows) into a
// PID -> process name map. Each row is one process:
//
//	"chrome.exe","14232","Console","1","120,344 K"
//
// encoding/csv is used rather than splitting on commas because the
// memory column contains commas.
func parseTasklist(out string) map[int]string {
	names := make(map[int]string)

	r := csv.NewReader(strings.NewReader(out))
	r.FieldsPerRecord = -1 // rows vary in length; we only need the first two
	r.LazyQuotes = true

	for {
		record, err := r.Read()
		if err != nil {
			// io.EOF ends the list; a malformed row also ends this
			// best-effort pass. Partial names are more useful than none.
			break
		}
		if len(record) < 2 {
			continue // e.g. "INFO: No tasks are running..."
		}
		pid, err := strconv.Atoi(strings.TrimSpace(record[1]))
		if err != nil || pid < 0 {
			continue
		}
		names[pid] = strings.TrimSpace(record[0])
	}
	return names
}

// socketInode extracts 123456 from "socket:[123456]", the form a Linux
// file descriptor takes when it refers to a socket.
//
// It lives here, not in the Linux adapter, so it can be tested on every
// platform: a parser that cannot be run by the test suite tends not to
// be run at all.
func socketInode(target string) (string, bool) {
	const prefix = "socket:["
	if !strings.HasPrefix(target, prefix) || !strings.HasSuffix(target, "]") {
		return "", false
	}
	return target[len(prefix) : len(target)-1], true
}

// parseProcNetTCP reads /proc/net/tcp or /proc/net/tcp6 (Linux).
//
//	sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
//	 0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 123456 ...
//
// Addresses and ports are hexadecimal and the interface is written
// little-endian. State 0A is TCP_LISTEN. The inode in field 9 is how
// the kernel lets us find the owning process afterwards.
func parseProcNetTCP(out string) []Listener {
	var listeners []Listener
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue // header line and blank lines
		}
		if !strings.EqualFold(fields[3], "0A") {
			continue
		}
		local := fields[1]
		i := strings.LastIndex(local, ":")
		if i < 0 || i == len(local)-1 {
			continue
		}
		port, err := strconv.ParseInt(local[i+1:], 16, 32)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		listeners = append(listeners, Listener{
			Port:     int(port),
			Protocol: "tcp",
			inode:    fields[9],
		})
	}
	return listeners
}
