//go:build openbsd

package cmd

import (
	"testing"

	"github.com/kardianos/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderRCScript(t *testing.T) {
	script, err := renderRCScript("/usr/local/bin/netbird", []string{
		"service", "run", "--log-level", "info", "--daemon-addr", "unix:///var/run/netbird.sock",
	})
	require.NoError(t, err)

	assert.Contains(t, script, `daemon="/usr/local/bin/netbird"`, "daemon path")
	assert.Contains(t, script, `daemon_flags="service run --log-level info --daemon-addr unix:///var/run/netbird.sock"`, "daemon flags")
	assert.Contains(t, script, "rc_bg=YES", "daemon runs in the background")
	assert.Contains(t, script, "rc_cmd $1", "rc.subr entry point")
}

func TestRenderRCScriptRejectsUnsafeArguments(t *testing.T) {
	for _, arg := range []string{"has space", `quote"`, "$(id)", "a;b", "back`tick`", "a+b", ""} {
		_, err := renderRCScript("/usr/local/bin/netbird", []string{"service", "run", arg})
		assert.Error(t, err, "argument %q must be rejected", arg)
	}
}

func TestInstallRejectsInvalidServiceName(t *testing.T) {
	// rc.subr only accepts shell identifiers, and the check runs before the
	// script is written, so no rc.d file is left behind.
	for _, name := range []string{"netbird-dev", "1netbird", "../netbird", ""} {
		svc := &openbsdService{config: &service.Config{Name: name}}
		assert.Error(t, svc.Install(), "service name %q must be rejected", name)
	}
}
