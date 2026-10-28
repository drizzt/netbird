//go:build openbsd

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/kardianos/service"
)

const rcDir = "/etc/rc.d"

// rcSafeArg matches the arguments that can be placed in an rc.d script
// unquoted. rc.subr also matches the running daemon against its flags with
// pgrep, which breaks once the flags carry shell quoting or a regex operator
// other than the dot.
var rcSafeArg = regexp.MustCompile(`^[A-Za-z0-9_@%=:,./-]+$`)

// rcServiceName matches the names rc.subr and rcctl(8) accept for a service.
var rcServiceName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// kardianos/service has no OpenBSD backend; this one manages an rc.d script
// through rcctl(8).
func init() {
	service.ChooseSystem(openbsdSystem{})
}

type openbsdSystem struct{}

func (openbsdSystem) String() string { return "openbsd-rcctl" }

func (openbsdSystem) Detect() bool { return true }

func (openbsdSystem) Interactive() bool { return false }

func (openbsdSystem) New(i service.Interface, c *service.Config) (service.Service, error) {
	return &openbsdService{i: i, config: c}, nil
}

type openbsdService struct {
	i      service.Interface
	config *service.Config
}

func (s *openbsdService) String() string { return s.config.DisplayName }

func (s *openbsdService) Platform() string { return "openbsd-rcctl" }

func (s *openbsdService) scriptPath() string {
	return filepath.Join(rcDir, s.config.Name)
}

func (s *openbsdService) Install() error {
	if len(s.config.EnvVars) > 0 {
		return errors.New("service environment variables are not supported on OpenBSD")
	}
	if !rcServiceName.MatchString(s.config.Name) {
		return fmt.Errorf("service name %q is not valid for rc.d: use letters, digits and underscores", s.config.Name)
	}

	path := s.scriptPath()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("init script already exists: %s", path)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	script, err := renderRCScript(exe, s.config.Arguments)
	if err != nil {
		return err
	}

	// nolint:gosec // standard rc.d script permissions
	if err := os.WriteFile(path, []byte(script), 0o555); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return rcctl("enable", s.config.Name)
}

func (s *openbsdService) Uninstall() error {
	if err := rcctl("disable", s.config.Name); err != nil {
		return err
	}
	return os.Remove(s.scriptPath())
}

func (s *openbsdService) Start() error { return rcctl("start", s.config.Name) }

func (s *openbsdService) Stop() error { return rcctl("stop", s.config.Name) }

func (s *openbsdService) Restart() error { return rcctl("restart", s.config.Name) }

func (s *openbsdService) Status() (service.Status, error) {
	if _, err := os.Stat(s.scriptPath()); errors.Is(err, os.ErrNotExist) {
		return service.StatusStopped, service.ErrNotInstalled
	}

	err := exec.Command("rcctl", "check", s.config.Name).Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return service.StatusRunning, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return service.StatusStopped, nil
	default:
		return service.StatusUnknown, fmt.Errorf("rcctl check %s: %w", s.config.Name, err)
	}
}

func (s *openbsdService) Run() error {
	if err := s.i.Start(s); err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
	<-sigChan

	return s.i.Stop(s)
}

func (s *openbsdService) Logger(chan<- error) (service.Logger, error) {
	return service.ConsoleLogger, nil
}

func (s *openbsdService) SystemLogger(chan<- error) (service.Logger, error) {
	return service.ConsoleLogger, nil
}

func renderRCScript(exe string, args []string) (string, error) {
	for _, arg := range append([]string{exe}, args...) {
		if !rcSafeArg.MatchString(arg) {
			return "", fmt.Errorf("argument %q cannot be used in an rc.d script", arg)
		}
	}

	return fmt.Sprintf(`#!/bin/ksh

daemon=%q
daemon_flags=%q

. /etc/rc.d/rc.subr

rc_bg=YES
rc_reload=NO

rc_cmd $1
`, exe, strings.Join(args, " ")), nil
}

func rcctl(args ...string) error {
	if out, err := exec.Command("rcctl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("rcctl %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
