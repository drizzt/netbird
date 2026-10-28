//go:build openbsd

package configurer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunUnit(t *testing.T) {
	unit, err := TunUnit("tun255")
	require.NoError(t, err)
	assert.Equal(t, uint8(255), unit, "highest unit")

	// wireguard-go would open tun7 for tun007, and units above 255 wrap.
	for _, name := range []string{"tun256", "tun007", "tun", "tun-1", "wt0", "utun1"} {
		_, err := TunUnit(name)
		assert.Error(t, err, "name %q must be rejected", name)
	}
}
