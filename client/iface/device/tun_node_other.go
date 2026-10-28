//go:build (linux && !android) || freebsd

package device

func prepareTunNode(string) error {
	return nil
}
