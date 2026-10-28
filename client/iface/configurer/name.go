//go:build linux || windows || freebsd || js || wasip1

package configurer

// WgInterfaceDefault is a default interface name of Netbird
const WgInterfaceDefault = "wt0"

// ValidateInterfaceName accepts any name: the platform names the interface freely.
func ValidateInterfaceName(string) error {
	return nil
}
