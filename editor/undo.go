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
}

func (o *op) inverse() *op {
	inv := &op{kind: o.kind, line: o.line, col: o.col, text: o.text, new: o.new, curBefore: o.curAfter, curAfter: o.curBefore}
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
	undoS []*op
	redoS []*op
}

func (s *UndoStack) push(o *op) {
	if o.kind == opInsert && len(s.undoS) > 0 {
		last := s.undoS[len(s.undoS)-1]
		if last.kind == opInsert && last.line == o.line && o.col == last.col+len(last.text) {
			last.text = append(last.text, o.text...)
			last.curAfter = o.curAfter
			s.redoS = nil
			return
		}
	}
	s.undoS = append(s.undoS, o)
	s.redoS = nil
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
	s.redoS = append(s.redoS, o)
	return o
}

func (s *UndoStack) redo() *op {
	if len(s.redoS) == 0 {
		return nil
	}
	o := s.redoS[len(s.redoS)-1]
	s.redoS = s.redoS[:len(s.redoS)-1]
	s.undoS = append(s.undoS, o)
	return o
}
