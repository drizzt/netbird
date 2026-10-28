//go:build darwin || dragonfly

package systemops

import "golang.org/x/sys/unix"

const rtfWasCloned = unix.RTF_WASCLONED
