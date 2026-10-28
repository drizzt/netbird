//go:build openbsd

package configurer

import (
	"fmt"
	"strconv"
	"strings"
)

// WgInterfaceDefault is a default interface name of Netbird. OpenBSD tun(4)
// interfaces cannot be renamed, so the name must be tunN with N below 256.
const WgInterfaceDefault = "tun100"

// ValidateInterfaceName checks that name is a tun interface name.
func ValidateInterfaceName(name string) error {
	_, err := TunUnit(name)
	return err
}

// TunUnit returns the unit number of a tun interface name. wireguard-go parses
// the unit with %d, so tun007 would open tun7, and maps the device minor back
// to a name modulo 256: only the canonical tun0 to tun255 is accepted.
func TunUnit(name string) (uint8, error) {
	unit, err := strconv.ParseUint(strings.TrimPrefix(name, "tun"), 10, 8)
	if err != nil || name != "tun"+strconv.FormatUint(unit, 10) {
		return 0, fmt.Errorf("invalid interface name %s. Please use tun followed by a number from 0 to 255 on OpenBSD. e.g., tun100", name)
	}
	return uint8(unit), nil
}
