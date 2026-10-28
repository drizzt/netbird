//go:build openbsd

package system

import (
	"context"
	"os"
	"runtime"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/netbirdio/netbird/client/system/detect_cloud"
	"github.com/netbirdio/netbird/client/system/detect_platform"
	"github.com/netbirdio/netbird/version"
)

// UpdateStaticInfoAsync is a no-op on OpenBSD as there is no static info to update.
func UpdateStaticInfoAsync() {
	// do nothing
}

// GetInfo retrieves and parses the system information.
func GetInfo(ctx context.Context) *Info {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		log.Warnf("getInfo: %s", err)
	}
	sysName := unix.ByteSliceToString(utsname.Sysname[:])
	release := unix.ByteSliceToString(utsname.Release[:])

	env := Environment{
		Cloud:    detect_cloud.Detect(ctx),
		Platform: detect_platform.Detect(ctx),
	}

	systemHostname, _ := os.Hostname()

	addrs, err := networkAddresses()
	if err != nil {
		log.Warnf("failed to discover network addresses: %s", err)
	}

	return &Info{
		GoOS:             runtime.GOOS,
		Kernel:           sysName,
		Platform:         runtime.GOARCH,
		OS:               sysName,
		OSVersion:        release,
		Hostname:         extractDeviceName(ctx, systemHostname),
		CPUs:             runtime.NumCPU(),
		NetbirdVersion:   version.NetbirdVersion(),
		UIVersion:        extractUserAgent(ctx),
		KernelVersion:    release,
		NetworkAddresses: addrs,
		Environment:      env,
	}
}
