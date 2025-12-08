package naming

import (
	"math/big"
	mrand "math/rand"
	"testing"
)

func TestGenerateDeterministic(t *testing.T) {
	seed := mrand.New(mrand.NewSource(42))
	fakeRand := func(max *big.Int) (*big.Int, error) {
		return new(big.Int).Rand(seed, max), nil
	}

	got, err := generateWith(fakeRand)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	const want = "upbeat-summit"
	if got != want {
		t.Fatalf("Generate = %q, want %q", got, want)
	}
}
