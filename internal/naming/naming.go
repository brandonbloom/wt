package naming

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

var adjectives = []string{
	"amber", "bold", "brisk", "calm", "clever", "crisp", "curious", "daring", "eager", "early",
	"fierce", "gentle", "golden", "grand", "happy", "heroic", "honest", "ivory", "jolly", "keen",
	"lunar", "merry", "mint", "noble", "ocean", "peppy", "plucky", "quick", "quiet", "radial",
	"rapid", "ready", "shiny", "silky", "solar", "spry", "stout", "sunny", "swift", "tidy",
	"tranquil", "true", "urban", "vivid", "warm", "whimsical", "windy", "wise", "witty", "zesty",
	"atomic", "breezy", "cosmic", "dapper", "elegant", "frosty", "glossy", "humble", "kind",
	"lucid", "mellow", "neon", "pepper", "royal", "snappy", "stellar", "timely", "velvet", "zippy",
}

var nouns = []string{
	"acorn", "aurora", "bistro", "brook", "canoe", "cedar", "clover", "comet", "coral", "crown",
	"dawn", "ember", "falcon", "fjord", "flint", "flora", "forest", "galaxy", "harbor", "horizon",
	"isle", "ivory", "lagoon", "lantern", "lilac", "meadow", "meteor", "molecule", "monsoon",
	"nectar", "nebula", "opal", "orchid", "otter", "pebble", "pepper", "prairie", "quartz",
	"quill", "raven", "reef", "river", "saffron", "sage", "saturn", "sprout", "starlight", "summit",
	"sunrise", "tangent", "thistle", "thunder", "topaz", "tundra", "velvet", "violet", "walnut",
	"willow", "yonder", "zephyr", "zenith", "aurora", "basil", "cinder", "drift", "ember", "grove",
	"harvest", "lumen", "marble", "plume", "rover", "spire", "titan", "vista", "wren", "yarrow",
}

// Generate returns a hyphenated adjective-noun pair suitable for a worktree.
func Generate() (string, error) {
	adj, err := pick(adjectives)
	if err != nil {
		return "", err
	}
	noun, err := pick(nouns)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", adj, noun), nil
}

func pick(options []string) (string, error) {
	n := big.NewInt(int64(len(options)))
	i, err := rand.Int(rand.Reader, n)
	if err != nil {
		return "", err
	}
	return options[i.Int64()], nil
}
