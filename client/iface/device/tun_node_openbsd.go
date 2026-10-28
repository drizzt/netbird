//go:build openbsd

package device

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// prepareTunNode creates /dev/tunN when missing. A default install only ships
// /dev/tun0 to /dev/tun3, and wireguard-go opens the node rather than cloning.
func prepareTunNode(name string) error {
	unit, err := configurer.TunUnit(name)
	if err != nil {
		return err
	}

	path := "/dev/" + name
	if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return err
	}

	var ref unix.Stat_t
	if err := unix.Stat("/dev/tun0", &ref); err != nil {
		return fmt.Errorf("stat /dev/tun0: %w", err)
	}
	dev := unix.Mkdev(unix.Major(uint64(ref.Rdev)), uint32(unit))
	if err := unix.Mknod(path, unix.S_IFCHR|0o600, int(dev)); err != nil {
		return fmt.Errorf("mknod %s: %w", path, err)
	}
	return nil
}
