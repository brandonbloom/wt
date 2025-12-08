package naming

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"math/big"
	"strings"
	"sync"
)

// Words contains the curated adjective/noun pools.
type Words struct {
	Adjectives []string
	Nouns      []string
}

var (
	//go:embed adjectives.txt
	adjectiveData string
	//go:embed nouns.txt
	nounData string

	wordsOnce sync.Once
	words     Words
)

// Generate returns a hyphenated adjective-noun pair suitable for a worktree.
func Generate() (string, error) {
	return generateWith(cryptoRandInt)
}

func generateWith(randInt func(*big.Int) (*big.Int, error)) (string, error) {
	lists := getWords()

	adj := pick(randInt, lists.Adjectives)
	noun := pick(randInt, lists.Nouns)
	return fmt.Sprintf("%s-%s", adj, noun), nil
}

func cryptoRandInt(max *big.Int) (*big.Int, error) {
	return rand.Int(rand.Reader, max)
}

func getWords() Words {
	wordsOnce.Do(func() {
		words = Words{
			Adjectives: parseWordList(adjectiveData),
			Nouns:      parseWordList(nounData),
		}
	})
	return words
}

func parseWordList(data string) []string {
	lines := strings.Split(data, "\n")
	words := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		words = append(words, line)
	}
	return words
}

func pick(randInt func(*big.Int) (*big.Int, error), options []string) string {
	n := big.NewInt(int64(len(options)))
	i, err := randInt(n)
	if err != nil {
		panic(fmt.Sprintf("naming pick failed: %v", err))
	}
	return options[i.Int64()]
}
