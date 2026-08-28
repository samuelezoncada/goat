package editor

import (
	"fmt"
	"strings"

	"github.com/rivo/uniseg"
)

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// cleanErr strips the redundant path prefix from an os error, which the caller
// usually shows right next to it anyway.
func cleanErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// zwj is the zero-width joiner: it glues emoji into one grapheme cluster.
const zwj = '‍'

// widthCache memoizes rune widths for the BMP. runeWidth is called for every
// cell of every frame and from displayCol in inner loops, so it must not
// allocate: the uniseg lookup (which needs a string) happens at most once per
// distinct rune.
var widthCache [0x10000]int8

// runeWidth returns the number of terminal cells a rune occupies: 0 for
// combining marks and joiners, 2 for East Asian wide and most emoji.
func runeWidth(r rune) int {
	if r >= 0x20 && r < 0x7f { // printable ASCII: the overwhelmingly common case
		return 1
	}
	if r < 0x20 || r == 0x7f {
		// Control characters, tab included. uniseg reports width 0 for these,
		// but they must not be mistaken for cluster continuations; callers
		// expand tabs themselves before asking for a width.
		return 1
	}
	if r < 0 {
		return 1
	}
	if r < 0x10000 {
		// Stored as width+1 so that 0 keeps meaning "not computed yet".
		if c := widthCache[r]; c != 0 {
			return int(c) - 1
		}
		w := uniseg.StringWidth(string(r))
		if w < 0 {
			w = 0
		}
		if w > 126 {
			w = 126
		}
		widthCache[r] = int8(w + 1)
		return w
	}
	w := uniseg.StringWidth(string(r))
	if w < 0 {
		return 0
	}
	return w
}

// isRegionalIndicator reports whether r is one half of a flag emoji.
func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// clusterEnd returns the index just past the grapheme cluster starting at i,
// and the display width of that cluster. Editing and rendering step by cluster
// so a combining mark, a variation selector, a ZWJ emoji sequence or a flag is
// treated as one character rather than being split apart.
func clusterEnd(line []rune, i int) (int, int) {
	if i < 0 {
		i = 0
	}
	if i >= len(line) {
		return len(line), 0
	}
	base := line[i]
	w := runeWidth(base)
	// A flag is exactly two regional indicators.
	if isRegionalIndicator(base) && i+1 < len(line) && isRegionalIndicator(line[i+1]) {
		return i + 2, 2
	}
	end := i + 1
	for end < len(line) {
		r := line[end]
		if runeWidth(r) == 0 {
			// combining mark, variation selector, ZWJ, ...
			end++
			continue
		}
		if line[end-1] == zwj {
			// the emoji after a joiner belongs to the same cluster
			end++
			continue
		}
		break
	}
	if w == 0 {
		// A line starting with a stray combining mark still needs one cell so
		// the character is visible and the columns stay addressable.
		w = 1
	}
	return end, w
}

// clusterStart snaps col back to the first rune of the cluster containing it.
func clusterStart(line []rune, col int) int {
	if col <= 0 {
		return 0
	}
	if col > len(line) {
		return len(line)
	}
	i := 0
	for i < len(line) {
		end, _ := clusterEnd(line, i)
		if col < end {
			return i
		}
		if col == end {
			return col
		}
		i = end
	}
	return len(line)
}

// nextClusterCol returns the column just after the cluster at col.
func nextClusterCol(line []rune, col int) int {
	if col >= len(line) {
		return len(line)
	}
	end, _ := clusterEnd(line, clusterStart(line, col))
	if end <= col {
		return col + 1
	}
	return end
}

// prevClusterCol returns the column at the start of the cluster before col.
func prevClusterCol(line []rune, col int) int {
	if col <= 0 {
		return 0
	}
	if col > len(line) {
		col = len(line)
	}
	prev := 0
	for i := 0; i < len(line); {
		end, _ := clusterEnd(line, i)
		if end >= col {
			return i
		}
		prev = end
		i = end
	}
	return prev
}
