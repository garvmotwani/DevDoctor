//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// inspect reads Linux's own process filesystem. No external program is
// involved, which matters because `ss`, `netstat` and `lsof` are all
// optional packages on minimal images.
//
// The kernel is explicit about the two halves of this:
//
//	/proc/net/tcp, /proc/net/tcp6   -> listening sockets (with an inode)
//	/proc/<pid>/fd/<n>              -> "socket:[inode]" for that process
//
// Matching the socket inode to the process that holds it is the only
// supported way to attribute a port. There is no system-wide API that
// returns port and owner together.
//
// Permissions: a process can read the fd table of its own user's
// processes. For other users' processes (and for other namespaces) the
// directory reads fail; those sockets are still reported, just without
// a PID. That is a normal, documented limitation — not an error, and
// not a reason to ask for root.
func inspect() ([]Listener, error) {
	owners := socketOwners()

	var listeners []Listener
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			// tcp6 is absent on kernels built without IPv6. Only a
			// failure of both files means we learned nothing.
			continue
		}
		for _, l := range parseProcNetTCP(string(data)) {
			if pid, ok := owners[l.inode]; ok {
				l.PID = pid
				l.Process = processName(pid)
			}
			listeners = append(listeners, l)
		}
	}

	if listeners == nil && !anyReadable("/proc/net/tcp", "/proc/net/tcp6") {
		return nil, os.ErrPermission
	}
	return listeners, nil
}

// socketOwners builds inode -> PID by looking at every readable process.
// Processes that cannot be read are skipped silently; that is the
// permission boundary described above.
func socketOwners() map[string]int {
	owners := make(map[string]int)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return owners
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue // not a process directory
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			// Readlink turns the descriptor into "socket:[123456]" for
			// sockets and a real path for everything else.
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if inode, ok := socketInode(target); ok {
				owners[inode] = pid
			}
		}
	}
	return owners
}

// processName reads the short name the kernel records for a PID.
// /proc/<pid>/comm is truncated by the kernel to 15 characters, which
// is why it is a name and not a command line. Command lines live in
// /proc/<pid>/cmdline, but they are not read here: arguments routinely
// carry tokens and passwords, and a diagnostics report must not copy
// them into logs or JSON.
func processName(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func anyReadable(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
