package editor

import (
	"fmt"

	"github.com/rivo/uniseg"
)

// runeWidth returns the number of terminal cells a rune occupies.
func runeWidth(r rune) int {
	w := uniseg.StringWidth(string(r))
	if w <= 0 {
		return 1
	}
	return w
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
