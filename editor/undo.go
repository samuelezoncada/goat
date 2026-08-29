package editor

type opKind int

const (
	opInsert opKind = iota
	opDelete
	opReplace
	opLineRemove
	opLineInsert
)

// op is a single recorded edit. Positions refer to the buffer state at the
// time the op was created.
type op struct {
	kind      opKind
	line, col int
	text      []rune // inserted, deleted, or replaced (old) runes
	new       []rune // replacement text (opReplace)
	curBefore Pos
	curAfter  Pos
	mut       int // number of buffer mutations this op accounts for
	group     int // non-zero: undone/redone together with its group mates
}

func (o *op) inverse() *op {
	inv := &op{kind: o.kind, line: o.line, col: o.col, text: o.text, new: o.new, curBefore: o.curAfter, curAfter: o.curBefore, group: o.group}
	switch o.kind {
	case opInsert:
		inv.kind = opDelete
	case opDelete:
		inv.kind = opInsert
	case opReplace:
		inv.text, inv.new = o.new, o.text
	case opLineRemove:
		inv.kind = opLineInsert
	case opLineInsert:
		inv.kind = opLineRemove
	}
	return inv
}

type UndoStack struct {
	undoS  []*op
	redoS  []*op
	rev    int  // net buffer mutations currently applied (undoable + redoable)
	sealed bool // the top op must not absorb further edits (set on save)

	groupDepth int // >0 while a multi-op action is being recorded
	curGroup   int // group id assigned to ops recorded inside the action
	nextGroup  int
}

// maxUndo bounds the undo history so long sessions don't grow memory without
// limit; the oldest ops are dropped first.
const maxUndo = 2000

// cloneRunes copies rs so a recorded op owns its text: buffer arrays are
// rewritten in place by later edits, and an op that aliased one would silently
// change meaning.
func cloneRunes(rs []rune) []rune {
	if rs == nil {
		return nil
	}
	out := make([]rune, len(rs))
	copy(out, rs)
	return out
}

// begin starts a transaction: every op recorded until the matching end() is
// undone and redone as a single action. Calls nest.
func (s *UndoStack) begin() {
	s.groupDepth++
	if s.groupDepth == 1 {
		s.nextGroup++
		s.curGroup = s.nextGroup
	}
}

func (s *UndoStack) end() {
	if s.groupDepth > 0 {
		s.groupDepth--
	}
	if s.groupDepth == 0 {
		s.curGroup = 0
		// A transaction is a unit; the next edit must not merge into its last op.
		s.sealed = true
	}
}

// coalesce reports whether o can be merged into last, so a run of typing or
// backspacing collapses into one undo step.
func coalesce(last, o *op) bool {
	if last.group != 0 || o.group != 0 || last.line != o.line {
		return false
	}
	if last.kind != o.kind || containsNL(last.text) || containsNL(o.text) {
		return false
	}
	switch o.kind {
	case opInsert:
		// typing forward: "ab" + "c"
		return o.col == last.col+len(last.text)
	case opDelete:
		// backspace: deletions walking left from the same point
		if o.col+len(o.text) == last.col {
			return true
		}
		// delete-forward: repeated deletions at the same column
		return o.col == last.col
	}
	return false
}

func containsNL(rs []rune) bool {
	for _, r := range rs {
		if r == '\n' {
			return true
		}
	}
	return false
}

func (s *UndoStack) push(o *op) {
	o.text = cloneRunes(o.text)
	o.new = cloneRunes(o.new)
	o.group = s.curGroup
	if !s.sealed && len(s.undoS) > 0 {
		if last := s.undoS[len(s.undoS)-1]; coalesce(last, o) {
			switch {
			case o.kind == opInsert:
				last.text = append(last.text, o.text...)
			case o.col < last.col: // backspace: grows to the left
				last.text = append(cloneRunes(o.text), last.text...)
				last.col = o.col
			default: // delete-forward: grows to the right
				last.text = append(last.text, o.text...)
			}
			last.curAfter = o.curAfter
			last.mut++
			s.rev++
			s.redoS = nil
			return
		}
	}
	o.mut = 1
	s.undoS = append(s.undoS, o)
	s.rev++
	s.sealed = false
	s.trim()
	s.redoS = nil
}

// trim drops the oldest ops past the cap, whole groups at a time, without
// reslicing the front away (which would make every later push reallocate).
func (s *UndoStack) trim() {
	if len(s.undoS) <= maxUndo {
		return
	}
	drop := len(s.undoS) - maxUndo
	// Never split a group: extend the drop to the end of the oldest group.
	if g := s.undoS[drop-1].group; g != 0 {
		for drop < len(s.undoS) && s.undoS[drop].group == g {
			drop++
		}
	}
	if drop >= len(s.undoS) {
		drop = len(s.undoS) - 1
	}
	copy(s.undoS, s.undoS[drop:])
	for i := len(s.undoS) - drop; i < len(s.undoS); i++ {
		s.undoS[i] = nil
	}
	s.undoS = s.undoS[:len(s.undoS)-drop]
}

func (s *UndoStack) CanUndo() bool { return len(s.undoS) > 0 }
func (s *UndoStack) CanRedo() bool { return len(s.redoS) > 0 }

// undo pops the top op, applies its inverse, and returns the op to push to redo.
func (s *UndoStack) undo() *op {
	if len(s.undoS) == 0 {
		return nil
	}
	o := s.undoS[len(s.undoS)-1]
	s.undoS = s.undoS[:len(s.undoS)-1]
	s.rev -= o.mut
	s.redoS = append(s.redoS, o)
	return o
}

func (s *UndoStack) redo() *op {
	if len(s.redoS) == 0 {
		return nil
	}
	o := s.redoS[len(s.redoS)-1]
	s.redoS = s.redoS[:len(s.redoS)-1]
	s.rev += o.mut
	s.undoS = append(s.undoS, o)
	return o
}

// undoGroup pops one whole action: a single op, or every op recorded inside
// one transaction, newest first.
func (s *UndoStack) undoGroup() []*op {
	o := s.undo()
	if o == nil {
		return nil
	}
	out := []*op{o}
	if o.group != 0 {
		for len(s.undoS) > 0 && s.undoS[len(s.undoS)-1].group == o.group {
			out = append(out, s.undo())
		}
	}
	return out
}

// redoGroup re-applies one whole action, oldest op first.
func (s *UndoStack) redoGroup() []*op {
	o := s.redo()
	if o == nil {
		return nil
	}
	out := []*op{o}
	if o.group != 0 {
		for len(s.redoS) > 0 && s.redoS[len(s.redoS)-1].group == o.group {
			out = append(out, s.redo())
		}
	}
	return out
}
