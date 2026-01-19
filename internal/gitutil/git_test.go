package gitutil

import (
	"errors"
	"testing"
)

func TestIsMissingUpstreamError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "explicit no upstream",
			err:  errors.New("git rev-list --left-right --count @{u}...HEAD: exit status 128\nfatal: no upstream configured for branch 'doctor'"),
			want: true,
		},
		{
			name: "ambiguous @{u} revision",
			err:  errors.New("git rev-list --left-right --count @{u}...HEAD: exit status 128\nfatal: ambiguous argument '@{u}...HEAD': unknown revision or path not in the working tree."),
			want: true,
		},
		{
			name: "unrelated git failure",
			err:  errors.New("git rev-list: exit status 128\nfatal: bad revision 'HEAD^^'"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isMissingUpstreamError(tc.err)
			if got != tc.want {
				t.Fatalf("got %v, want %v (err=%v)", got, tc.want, tc.err)
			}
		})
	}
}
