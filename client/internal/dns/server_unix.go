//go:build (linux && !android) || freebsd || openbsd

package dns

func (s *DefaultServer) initialize() (manager hostManager, err error) {
	return newHostManager(s.wgInterface.Name())
}
