//go:build openbsd

package ipcauth

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerIdentityReportsConnectingProcess(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "peercred.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	client, err := net.Dial("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	server, err := ln.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	id, err := PeerIdentity(server)
	require.NoError(t, err)
	assert.Equal(t, uint32(os.Getuid()), id.UID, "uid of the connecting process")
	assert.Equal(t, uint32(os.Getgid()), id.GID, "gid of the connecting process")
	assert.Equal(t, int32(os.Getpid()), id.PID, "pid of the connecting process")
}
