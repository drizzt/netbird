//go:build netbsd || openbsd

package systemops

import "golang.org/x/sys/unix"

// rtfWasCloned marks routes cloned from a cloning route, named RTF_CLONED here.
const rtfWasCloned = unix.RTF_CLONED
