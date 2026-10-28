//go:build openbsd

package device

import (
	"fmt"
	"net"
	"os/exec"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/iface/wgaddr"
)

type wgLink struct {
	name string
}

func newWGLink(name string) *wgLink {
	return &wgLink{name: name}
}

// assignAddr sets the overlay address on the point-to-point tun interface. tun(4)
// installs no network route for a v4 netmask, so the overlay route is added
// explicitly; the kernel removes it when the interface is destroyed.
func (l *wgLink) assignAddr(address *wgaddr.Address) error {
	ip := address.IP.String()
	mask := net.IP(net.CIDRMask(address.Network.Bits(), 32)).String()

	log.Infof("assign addr %s mask %s to %s interface", ip, mask, l.name)
	if out, err := exec.Command("ifconfig", l.name, "inet", ip, ip, "netmask", mask, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("add v4 address: %s: %w", out, err)
	}

	// Match the gateway so the delete only drops a route this interface added,
	// never a route to the same network through another interface.
	network := address.Network.Masked().String()
	if out, err := exec.Command("route", "-qn", "delete", "-inet", network, ip).CombinedOutput(); err != nil {
		log.Debugf("no previous route %s to delete: %s: %v", network, out, err)
	}
	if out, err := exec.Command("route", "-n", "add", "-inet", network, "-iface", ip).CombinedOutput(); err != nil {
		return fmt.Errorf("add route %s via %s: %s: %w", network, l.name, out, err)
	}

	if address.HasIPv6() {
		log.Infof("assign IPv6 addr %s to %s interface", address.IPv6String(), l.name)
		if out, err := exec.Command("ifconfig", l.name, "inet6", address.IPv6String()).CombinedOutput(); err != nil {
			log.Warnf("failed to assign IPv6 address %s to %s, continuing v4-only: %s: %v", address.IPv6String(), l.name, out, err)
			address.ClearIPv6()
		}
	}

	return nil
}
