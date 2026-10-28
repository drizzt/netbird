//go:build openbsd

package dns

import (
	"encoding/binary"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestBuildDNSProposal(t *testing.T) {
	server := netip.MustParseAddr("100.64.0.1")
	msg, err := buildDNSProposal(7, unix.AF_INET, []netip.Addr{server})
	require.NoError(t, err)

	// rt_msghdr (96 bytes) followed by sockaddr_rtdns (130 bytes) padded to 136.
	require.Len(t, msg, unix.SizeofRtMsghdr+136, "message length")
	assert.Equal(t, uint16(len(msg)), binary.NativeEndian.Uint16(msg[0:]), "rtm_msglen")
	assert.Equal(t, byte(unix.RTM_VERSION), msg[2], "rtm_version")
	assert.Equal(t, byte(unix.RTM_PROPOSAL), msg[3], "rtm_type")
	assert.Equal(t, uint16(unix.SizeofRtMsghdr), binary.NativeEndian.Uint16(msg[4:]), "rtm_hdrlen")
	assert.Equal(t, uint16(7), binary.NativeEndian.Uint16(msg[6:]), "rtm_index")
	assert.Equal(t, byte(rtpProposalStatic), msg[10], "rtm_priority")
	assert.Equal(t, uint32(unix.RTA_DNS), binary.NativeEndian.Uint32(msg[12:]), "rtm_addrs")

	sa := msg[unix.SizeofRtMsghdr:]
	assert.Equal(t, byte(6), sa[0], "sr_len covers one v4 address")
	assert.Equal(t, byte(unix.AF_INET), sa[1], "sr_family")
	assert.Equal(t, server.AsSlice(), sa[2:6], "sr_dns")
}

func TestBuildDNSProposalWithdraw(t *testing.T) {
	msg, err := buildDNSProposal(7, unix.AF_INET6, nil)
	require.NoError(t, err)

	sa := msg[unix.SizeofRtMsghdr:]
	assert.Equal(t, byte(2), sa[0], "empty proposal has no addresses")
	assert.Equal(t, byte(unix.AF_INET6), sa[1], "sr_family")
}

func TestIsProposalSolicit(t *testing.T) {
	proposal, err := buildDNSProposal(7, unix.AF_INET, []netip.Addr{netip.MustParseAddr("100.64.0.1")})
	require.NoError(t, err)
	assert.False(t, isProposalSolicit(proposal), "a regular proposal is not a solicitation")

	// resolvd solicits with a bare rt_msghdr carrying RTP_PROPOSAL_SOLICIT.
	solicit := make([]byte, unix.SizeofRtMsghdr)
	solicit[2] = unix.RTM_VERSION
	solicit[3] = unix.RTM_PROPOSAL
	solicit[10] = rtpProposalSolicit
	assert.True(t, isProposalSolicit(solicit), "resolvd solicitation")

	assert.False(t, isProposalSolicit(solicit[:4]), "truncated message")

	solicit[3] = unix.RTM_ADD
	assert.False(t, isProposalSolicit(solicit), "other message type")
}

func TestBuildDNSProposalFamilyMismatch(t *testing.T) {
	_, err := buildDNSProposal(7, unix.AF_INET6, []netip.Addr{netip.MustParseAddr("100.64.0.1")})
	assert.Error(t, err)
}

func TestReadNameservers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	content := "nameserver 192.168.1.1 # resolvd: em0\n" +
		"nameserver 100.64.0.1 # resolvd: tun100\n" +
		"nameserver fe80::1%em0 # resolvd: em0\n" +
		"search example.org\n" +
		"lookup file bind\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	got := readNameservers(path, netip.MustParseAddr("100.64.0.1"))
	assert.Equal(t, []netip.Addr{
		netip.MustParseAddr("192.168.1.1"),
		netip.MustParseAddr("fe80::1%em0"),
	}, got, "NetBird resolver must be excluded")
}
