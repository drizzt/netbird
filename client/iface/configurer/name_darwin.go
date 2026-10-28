//go:build darwin

package configurer

import (
	"fmt"
	"strings"
)

// WgInterfaceDefault is a default interface name of Netbird
const WgInterfaceDefault = "utun100"

// ValidateInterfaceName checks that name is a utun interface name.
func ValidateInterfaceName(name string) error {
	if strings.HasPrefix(name, "utun") {
		return nil
	}
	return fmt.Errorf("invalid interface name %s. Please use the prefix utun followed by a number on MacOS. e.g., utun1 or utun199", name)
}
