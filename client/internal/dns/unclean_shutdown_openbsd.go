//go:build openbsd

package dns

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

// ShutdownState records the interface whose resolvd proposals a crashed daemon
// may have left behind.
type ShutdownState struct {
	WgIface string
}

// Name returns the state manager key of the DNS shutdown state.
func (s *ShutdownState) Name() string {
	return "dns_state"
}

// Cleanup withdraws proposals left behind by a crashed daemon. resolvd already
// dropped them if the interface is gone.
func (s *ShutdownState) Cleanup() error {
	if s.WgIface == "" {
		return nil
	}
	if _, err := net.InterfaceByName(s.WgIface); err != nil {
		log.Debugf("interface %s is gone, resolvd dropped its proposals: %v", s.WgIface, err)
		return nil
	}
	if err := withdrawAllProposals(s.WgIface); err != nil {
		return fmt.Errorf("restore unclean shutdown dns: %w", err)
	}
	return nil
}
