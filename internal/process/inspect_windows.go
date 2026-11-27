//go:build windows

package process

import "os/exec"

// inspect asks Windows for the listening TCP sockets and then for a
// PID -> name table.
//
// Windows has no readable /proc. The information lives in the IP Helper
// API (GetExtendedTcpTable), which Go's standard library does not
// expose without cgo. The two command-line tools Windows ships solve
// the same problem:
//
//	netstat -ano -p tcp   -> every socket, with the owning PID
//	tasklist /FO CSV /NH  -> every process, with its name
//
// Both are read-only queries. No shell is involved: each program is run
// directly with an argument vector, so nothing can be interpreted as a
// command string.
//
// tasklist is best effort on purpose. A standard user sees the names of
// their own processes but not of services or other users' processes;
// the PID is still accurate in that case, so the port stays attributed
// to a PID even when the name is unavailable.
func inspect() ([]Listener, error) {
	netstatOut, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil, err
	}

	listeners := parseNetstat(string(netstatOut))

	if out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output(); err == nil {
		names := parseTasklist(string(out))
		for i := range listeners {
			listeners[i].Process = names[listeners[i].PID]
		}
	}

	return listeners, nil
}
