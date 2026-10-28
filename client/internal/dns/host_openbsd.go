//go:build openbsd

package dns

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/netbirdio/netbird/client/internal/statemanager"
)

const defaultResolvConfPath = "/etc/resolv.conf"

// Constants from net/route.h that x/sys does not define.
const (
	// rtpProposalStatic is RTP_PROPOSAL_STATIC.
	rtpProposalStatic = 57
	// rtpProposalSolicit is RTP_PROPOSAL_SOLICIT, sent by resolvd on startup
	// to ask every proposer to resend its proposals.
	rtpProposalSolicit = 62
	// routeMsgFilter is ROUTE_MSGFILTER, the route socket message type filter.
	routeMsgFilter = 1
	// rtdnsLen is RTDNS_LEN, the size of sockaddr_rtdns.sr_dns.
	rtdnsLen = 128
)

// resolvdConfigurator hands the NetBird resolver to resolvd(8) as a route
// proposal bound to the NetBird interface. resolvd drops the proposal when the
// interface goes away, so a crashed daemon cannot leave the host DNS broken.
// resolvd ignores search domains in proposals, so none are configured.
type resolvdConfigurator struct {
	ifaceName string

	mu                  sync.Mutex
	proposed            netip.Addr
	originalNameservers []netip.Addr
	solicit             *solicitListener
}

func newHostManager(ifaceName string) (*resolvdConfigurator, error) {
	return &resolvdConfigurator{ifaceName: ifaceName}, nil
}

func (r *resolvdConfigurator) supportCustomPort() bool {
	return false
}

func (r *resolvdConfigurator) string() string {
	return "resolvd"
}

func (r *resolvdConfigurator) applyDNSConfig(config HostDNSConfig, stateManager *statemanager.Manager) error {
	ifIndex, err := getInterfaceIndex(r.ifaceName)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.originalNameservers == nil {
		r.originalNameservers = readNameservers(defaultResolvConfPath, config.ServerIP)
	}

	if err := proposeNameserver(ifIndex, config.ServerIP); err != nil {
		return err
	}
	r.proposed = config.ServerIP

	if r.solicit == nil {
		listener, err := startSolicitListener(r.resendProposal)
		if err != nil {
			log.Warnf("failed to listen for resolvd solicitations, a resolvd restart drops the NetBird nameserver: %v", err)
		} else {
			r.solicit = listener
		}
	}

	if err := stateManager.UpdateState(&ShutdownState{WgIface: r.ifaceName}); err != nil {
		log.Errorf("failed to update shutdown state: %s", err)
	}

	log.Infof("proposed nameserver %s to resolvd on %s", config.ServerIP, r.ifaceName)
	return nil
}

func (r *resolvdConfigurator) restoreHostDNS() error {
	r.mu.Lock()
	listener := r.solicit
	r.solicit = nil
	r.proposed = netip.Addr{}
	r.mu.Unlock()

	// The listener takes mu to answer a solicitation, so wait for it unlocked.
	if listener != nil {
		listener.stop()
	}

	return withdrawAllProposals(r.ifaceName)
}

func (r *resolvdConfigurator) getOriginalNameservers() []netip.Addr {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.originalNameservers)
}

// resendProposal answers a resolvd solicitation with the current proposal.
func (r *resolvdConfigurator) resendProposal() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.proposed.IsValid() {
		return
	}

	ifIndex, err := getInterfaceIndex(r.ifaceName)
	if err != nil {
		log.Warnf("failed to answer resolvd solicitation: %v", err)
		return
	}
	if err := proposeNameserver(ifIndex, r.proposed); err != nil {
		log.Warnf("failed to answer resolvd solicitation: %v", err)
		return
	}
	log.Debugf("re-proposed nameserver %s to resolvd on %s", r.proposed, r.ifaceName)
}

// proposeNameserver proposes server for its address family and withdraws the
// other family, so resolvd holds exactly one NetBird nameserver.
func proposeNameserver(ifIndex int, server netip.Addr) error {
	if err := sendDNSProposals(ifIndex, server); err != nil {
		return fmt.Errorf("propose nameserver %s: %w", server, err)
	}
	return nil
}

// withdrawAllProposals clears proposals of both families for the interface.
func withdrawAllProposals(ifaceName string) error {
	ifIndex, err := getInterfaceIndex(ifaceName)
	if err != nil {
		return err
	}
	if err := sendDNSProposals(ifIndex, netip.Addr{}); err != nil {
		return fmt.Errorf("withdraw nameserver proposals: %w", err)
	}
	return nil
}

func addrFamily(addr netip.Addr) int {
	if addr.Is4() {
		return unix.AF_INET
	}
	return unix.AF_INET6
}

// readNameservers returns the nameservers in a resolv.conf, skipping exclude.
func readNameservers(path string, exclude netip.Addr) []netip.Addr {
	file, err := os.Open(path)
	if err != nil {
		log.Warnf("failed to read original nameservers from %s: %v", path, err)
		return []netip.Addr{}
	}
	defer file.Close()

	servers := []netip.Addr{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr, err := netip.ParseAddr(fields[1])
		if err != nil {
			log.Debugf("skip unparsable nameserver %q: %v", fields[1], err)
			continue
		}
		addr = addr.Unmap()
		if addr != exclude {
			servers = append(servers, addr)
		}
	}
	return servers
}

// sendDNSProposals sends one proposal per address family: server for its own
// family and a withdrawal for the other. An invalid server withdraws both.
func sendDNSProposals(ifIndex int, server netip.Addr) error {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.AF_UNSPEC)
	if err != nil {
		return fmt.Errorf("open route socket: %w", err)
	}
	defer func() {
		if err := unix.Close(fd); err != nil {
			log.Debugf("failed to close route socket: %v", err)
		}
	}()

	for _, family := range []int{unix.AF_INET, unix.AF_INET6} {
		var servers []netip.Addr
		if server.IsValid() && addrFamily(server) == family {
			servers = []netip.Addr{server}
		}
		msg, err := buildDNSProposal(ifIndex, family, servers)
		if err != nil {
			return err
		}
		if _, err := unix.Write(fd, msg); err != nil {
			return fmt.Errorf("write route proposal: %w", err)
		}
	}
	return nil
}

// solicitListener watches the route socket for resolvd's solicitation. resolvd
// keeps learned proposals only in memory, so after a restart it relearns them
// from the answers to the solicitation it sends on startup.
type solicitListener struct {
	file *os.File
	done chan struct{}
}

func startSolicitListener(onSolicit func()) (*solicitListener, error) {
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.AF_UNSPEC)
	if err != nil {
		return nil, fmt.Errorf("open route socket: %w", err)
	}
	if err := unix.SetsockoptInt(fd, unix.AF_ROUTE, routeMsgFilter, 1<<unix.RTM_PROPOSAL); err != nil {
		if closeErr := unix.Close(fd); closeErr != nil {
			log.Debugf("failed to close route socket: %v", closeErr)
		}
		return nil, fmt.Errorf("filter route socket: %w", err)
	}

	// A non-blocking fd goes through the runtime poller, so Close unblocks Read.
	l := &solicitListener{
		file: os.NewFile(uintptr(fd), "route"),
		done: make(chan struct{}),
	}
	go l.run(onSolicit)
	return l, nil
}

func (l *solicitListener) run(onSolicit func()) {
	defer close(l.done)

	buf := make([]byte, os.Getpagesize())
	for {
		n, err := l.file.Read(buf)
		if err != nil {
			if !errors.Is(err, os.ErrClosed) {
				log.Warnf("stopped listening for resolvd solicitations: %v", err)
			}
			return
		}
		if isProposalSolicit(buf[:n]) {
			onSolicit()
		}
	}
}

func (l *solicitListener) stop() {
	if err := l.file.Close(); err != nil {
		log.Debugf("failed to close route socket: %v", err)
	}
	<-l.done
}

// isProposalSolicit reports whether msg is an RTM_PROPOSAL asking every
// proposer to resend its proposals.
func isProposalSolicit(msg []byte) bool {
	if len(msg) < unix.SizeofRtMsghdr {
		return false
	}
	hdr := (*unix.RtMsghdr)(unsafe.Pointer(&msg[0]))
	return hdr.Version == unix.RTM_VERSION && hdr.Type == unix.RTM_PROPOSAL && hdr.Priority == rtpProposalSolicit
}

// buildDNSProposal encodes an RTM_PROPOSAL carrying a sockaddr_rtdns, laid out
// like iked's vroute_dodns. An empty server list withdraws the proposal of that
// family for the interface.
func buildDNSProposal(ifIndex, family int, servers []netip.Addr) ([]byte, error) {
	var dns []byte
	for _, s := range servers {
		if addrFamily(s) != family {
			return nil, fmt.Errorf("nameserver %s does not match address family %d", s, family)
		}
		dns = append(dns, s.AsSlice()...)
	}
	if len(dns) > rtdnsLen {
		return nil, fmt.Errorf("too many nameservers: %d", len(servers))
	}

	const saLen = 2 + rtdnsLen
	const saPadded = (saLen + 7) &^ 7

	hdr := unix.RtMsghdr{
		Msglen:   uint16(unix.SizeofRtMsghdr + saPadded),
		Version:  unix.RTM_VERSION,
		Type:     unix.RTM_PROPOSAL,
		Hdrlen:   unix.SizeofRtMsghdr,
		Index:    uint16(ifIndex),
		Priority: rtpProposalStatic,
		Addrs:    unix.RTA_DNS,
		Flags:    unix.RTF_UP,
	}

	msg := make([]byte, int(hdr.Msglen))
	copy(msg, unsafe.Slice((*byte)(unsafe.Pointer(&hdr)), unix.SizeofRtMsghdr))

	sa := msg[unix.SizeofRtMsghdr:]
	sa[0] = byte(2 + len(dns))
	sa[1] = byte(family)
	copy(sa[2:], dns)

	return msg, nil
}
