package editor

import "testing"

func TestVerticalMovePreservesColumn(t *testing.T) {
	tb := newTestTab("aaaaaaaaaaaa\nexit\nbbbbbbbbbbbb")
	tb.cur = Pos{0, 8}
	tb.destCol = 8
	tb.moveDown()
	if tb.cur.Col != 4 {
		t.Fatalf("after first Down col=%d want 4 (short line clamps)", tb.cur.Col)
	}
	if tb.destCol != 8 {
		t.Fatalf("destCol lost: %d want 8", tb.destCol)
	}
	tb.moveDown()
	if tb.cur.Col != 8 {
		t.Fatalf("after second Down col=%d want 8 (restored)", tb.cur.Col)
	}
	tb.moveUp()
	if tb.cur.Col != 4 {
		t.Fatalf("after Up col=%d want 4", tb.cur.Col)
	}
	tb.moveUp()
	if tb.cur.Col != 8 {
		t.Fatalf("after second Up col=%d want 8", tb.cur.Col)
	}
}

func TestHorizontalMoveResetsDestCol(t *testing.T) {
	tb := newTestTab("aaaaaaaaaaaa\nexit")
	tb.cur = Pos{0, 8}
	tb.destCol = 8
	tb.moveDown() // clamps to 4, destCol stays 8
	tb.moveLeft() // horizontal move: destCol follows cursor
	if tb.destCol != 3 {
		t.Fatalf("destCol=%d want 3", tb.destCol)
	}
	tb.moveUp()
	if tb.cur.Col != 3 {
		t.Fatalf("Up col=%d want 3", tb.cur.Col)
	}
}

func TestTypingSyncsDestCol(t *testing.T) {
	tb := newTestTab("aaaa\nbb")
	tb.cur = Pos{0, 1}
	tb.destCol = 1
	tb.insertRune('x') // cur.Col 2
	if tb.destCol != 2 {
		t.Fatalf("destCol=%d want 2 after typing", tb.destCol)
	}
	tb.moveDown()
	if tb.cur.Col != 2 {
		t.Fatalf("Down col=%d want 2", tb.cur.Col)
	}
}
