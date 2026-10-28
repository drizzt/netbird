//go:build openbsd

package ipcauth

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/unix"
)

// sockpeercred mirrors struct sockpeercred from sys/socket.h.
type sockpeercred struct {
	UID uint32
	GID uint32
	PID int32
}

// Fail the build if unix.Timeval cannot hold a sockpeercred (see PeerIdentity).
var _ [unsafe.Sizeof(unix.Timeval{}) - unsafe.Sizeof(sockpeercred{})]struct{}

// PeerIdentity reads the kernel-authenticated identity of the process on the
// other end of a Unix socket via SO_PEERCRED. The credentials are recorded by
// the kernel at connect() time and cannot be changed by the caller.
//
// x/sys has no SO_PEERCRED helper for OpenBSD, and raw getsockopt syscalls
// are refused there (Go must go through libc), so the option is read through
// GetsockoptTimeval, which passes a buffer large enough for sockpeercred.
func PeerIdentity(conn net.Conn) (Identity, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return Identity{}, fmt.Errorf("connection is not a unix socket: %T", conn)
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return Identity{}, fmt.Errorf("raw conn: %w", err)
	}

	var buf *unix.Timeval
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		buf, credErr = unix.GetsockoptTimeval(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Identity{}, fmt.Errorf("control raw conn: %w", err)
	}
	if credErr != nil {
		return Identity{}, fmt.Errorf("read SO_PEERCRED: %w", credErr)
	}

	cred := (*sockpeercred)(unsafe.Pointer(buf))
	return Identity{UID: cred.UID, GID: cred.GID, PID: cred.PID}, nil
}
