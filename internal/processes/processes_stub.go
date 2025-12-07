//go:build !linux && !darwin

package processes

func listNative(int) ([]Process, error) {
	return nil, ErrUnsupported
}
