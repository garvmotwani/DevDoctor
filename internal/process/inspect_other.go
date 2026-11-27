//go:build !windows && !linux

package process

// inspect is the honest answer for macOS and everything else: this
// build cannot see listening sockets yet.
//
// macOS would need either `lsof -nP -iTCP -sTCP:LISTEN` (not installed
// by default on current releases) or the libproc API, which requires
// cgo. Rather than ship code that silently claims "no ports in use" on
// a platform it never tested, DevDoctor reports the gap as a limitation
// through SnapshotOf and keeps the rest of the scan working.
func inspect() ([]Listener, error) {
	return nil, ErrUnsupported
}
