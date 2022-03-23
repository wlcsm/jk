package main

import (
	"strings"
	"unicode/utf8"
)

type SDK interface {
	InsertChar(c rune)
	DeleteChar()
	DeleteRow(at int)
	Find() error

	// Delete until the rune
	DeleteUntil(s rune)
	// Search to the left until the predicate matches
	FindLeft(f func(s rune) bool) (int, bool)
	// Search to the left for a specific rune
	FindRuneLeft(r rune) (int, bool)
	// Search to the left until the predicate matches
	FindRight(func(r rune) bool) (int, bool)
	// Search to the right for a specific rune
	FindRuneRight(s rune) (int, bool)

	// Set the absolute position of the cursor's y (wrapped)
	SetPosY(y int)
	SetPosMaxY()
	// Set the absolute position of the cursor's x (wrapped)
	SetPosX(x int)
	SetPosMaxX()

	// Set the absolute position of the cursor's x (wrapped)
	WrapCursorX()
	// Wrap the cursor to keep it in bounds
	WrapCursory()

	SetRelativePosY(y int)
	SetRelativePosX(x int)
	SetMode(m EditorMode)

	SetRow(at int, chars string)
	InsertRow(at int, chars string)

	CX() int
	CY() int
}

func (row *Row) insertChar(at int, c rune) {
	if at < 0 || at > len(row.chars) {
		at = len(row.chars)
	}
	row.chars = append(row.chars, 0) // make room
	copy(row.chars[at+1:], row.chars[at:])
	row.chars[at] = c
}

func (row *Row) appendChars(chars []rune) {
	row.chars = append(row.chars, chars...)
}

func (row *Row) deleteChar(at int) {
	if at < 0 || at >= len(row.chars) {
		return
	}
	row.chars = append(row.chars[:at], row.chars[at+1:]...)
}

func (e *Editor) InsertChar(c rune) {
	if e.cy == len(e.rows) {
		e.InsertRow(len(e.rows), "")
	}
	row := e.rows[e.cy]
	row.insertChar(e.cx, c)
	e.updateRow(row)
	e.cx++
	e.dirty++
}

func (e *Editor) DeleteChar() {
	if e.cy == len(e.rows) {
		return
	}
	if e.cx == 0 && e.cy == 0 {
		return
	}
	row := e.rows[e.cy]
	if e.cx > 0 {
		row.deleteChar(e.cx - 1)
		e.updateRow(row)
		e.cx--
		e.dirty++
	} else {
		prevRow := e.rows[e.cy-1]
		e.cx = len(prevRow.chars)
		prevRow.appendChars(row.chars)
		e.updateRow(prevRow)
		e.DeleteRow(e.cy)
		e.cy--
	}
}

func (e *Editor) DeleteRow(at int) {
	if at < 0 || at >= len(e.rows) {
		return
	}

	e.rows = append(e.rows[:at], e.rows[at+1:]...)
	for i := at; i < len(e.rows); i++ {
		e.rows[i].idx--
	}

	e.dirty++
	e.WrapCursorY()
	e.WrapCursorX()
}

/*** find ***/

func (e *Editor) Find() error {
	savedCx := e.cx
	savedCy := e.cy
	savedColOffset := e.colOffset
	savedRowOffset := e.rowOffset

	lastMatchRowIndex := -1 // remember the last match row
	searchDirection := 1    // 1 = forward, -1 = backward

	savedHlRowIndex := -1
	var savedHl []SyntaxHL

	onKeyPress := func(query string, k Key) {
		if len(savedHl) > 0 {
			copy(e.rows[savedHlRowIndex].hl, savedHl)
			savedHl = nil
		}
		switch k {
		case keyEnter, Key('\x1b'):
			lastMatchRowIndex = -1
			searchDirection = 1
			return
		case keyArrowRight, keyArrowDown:
			searchDirection = 1
		case keyArrowLeft, keyArrowUp:
			searchDirection = -1
		default:
			// unless an arrow key was pressed, we'll reset.
			lastMatchRowIndex = -1
			searchDirection = 1
		}

		if lastMatchRowIndex == -1 {
			searchDirection = 1
		}

		current := lastMatchRowIndex

		// search for query and set e.cy, e.cx, e.rowOffset values.
		for i := 0; i < len(e.rows); i++ {
			current += searchDirection
			switch current {
			case -1:
				current = len(e.rows) - 1
			case len(e.rows):
				current = 0
			}

			row := e.rows[current]
			rx := strings.Index(row.render, query)
			if rx != -1 {
				lastMatchRowIndex = current
				e.cy = current
				e.cx = rowRxToCx(row, rx)
				// set rowOffset to bottom so that the next scroll() will scroll
				// upwards and the matching line will be at the top of the screen
				e.rowOffset = len(e.rows)
				// highlight the matched string
				savedHlRowIndex = current
				savedHl = make([]SyntaxHL, len(row.hl))
				copy(savedHl, row.hl)
				for i := 0; i < utf8.RuneCountInString(query); i++ {
					row.hl[rx+i] = hlMatch
				}
				break
			}
		}
	}

	_, canceled, err := e.Prompt(
		"Search: %s (ESC = cancel | Enter = confirm | Arrows = prev/next)",
		onKeyPress,
	)
	// restore cursor position when the user cancels search
	if canceled {
		e.cx = savedCx
		e.cy = savedCy
		e.colOffset = savedColOffset
		e.rowOffset = savedRowOffset
	}
	return err
}

func (e *Editor) SetRow(at int, chars string) {
	if at < 0 || at > len(e.rows) {
		return
	}

	e.rows[at].chars = []rune(chars)
	e.updateRow(e.rows[at])

	// Make sure to wrap the cursor
	if e.cy == at {
		RepositionCursor()
	}
}

func (e *Editor) InsertRow(at int, chars string) {
	if at < 0 || at > len(e.rows) {
		return
	}
	row := &Row{chars: []rune(chars)}
	row.idx = at
	if at > 0 {
		row.hasUnclosedComment = e.rows[at-1].hasUnclosedComment
	}
	e.updateRow(row)

	// grow the buffer
	e.rows = append(e.rows, &Row{})
	copy(e.rows[at+1:], e.rows[at:])
	for i := at + 1; i < len(e.rows); i++ {
		e.rows[i].idx++
	}
	e.rows[at] = row
}

func (e *Editor) DeleteUntil(s rune) {
	i, _ := e.FindRuneLeft(s)

	row := e.rows[e.cy]
	row.chars = append(row.chars[:i], row.chars[e.cx:]...)
	e.cx = i

	e.updateRow(row)
}

func (e *Editor) FindLeft(f func(s rune) bool) (int, bool) {
	row := e.rows[e.cy]

	// Find the first instance of the rune starting at the cursor
	for i := e.cx - 1; i >= 0; i-- {
		if f(row.chars[i]) {
			return i, true
		}
	}

	return 0, false
}

func (e *Editor) FindRuneLeft(r rune) (int, bool) {
	return e.FindLeft(func(s rune) bool { return r == s })
}

func (e *Editor) FindRuneRight(s rune) (int, bool) {
	return e.FindRight(func(r rune) bool { return r == s })
}

func (e *Editor) FindRight(f func(s rune) bool) (int, bool) {
	row := e.rows[e.cy]

	// Find the first instance of the rune starting at the cursor
	for i := e.cx; i < len(row.chars); i++ {
		if f(row.chars[i]) {
			return i, true
		}
	}

	return len(row.chars), false
}

func (e *Editor) SetPosY(y int) {
	e.cy = y
	e.WrapCursorY()
}

func (e *Editor) SetPosMaxY() {
	e.cy = len(e.rows)
	e.WrapCursorY()
}

func (e *Editor) SetPosMaxX() {
	e.WrapCursorY()

	e.cx = len(e.rows[e.cy].chars)
}

func (e *Editor) WrapCursorX() {
	if e.cx < 0 {
		e.cx = 0
		return
	}

	if len(e.rows) == 0 {
		e.cx = 0
		return
	}

	if len(e.rows[e.cy].chars) == 0 {
		e.cx = 0
		return
	}

	if e.cx >= len(e.rows[e.cy].chars) {
		e.cx = len(e.rows[e.cy].chars)
	}
}

func (e *Editor) WrapCursorY() {
	if e.cy < 0 {
		e.cy = 0
		return
	}

	if len(e.rows) == 0 {
		e.cy = 0
		return
	}

	if e.cy >= len(e.rows) {
		e.cy = len(e.rows) - 1
	}
}

func (e *Editor) SetPosX(x int) {
	e.cx = x
	e.WrapCursorX()
}

func (e *Editor) SetRelativePosY(y int) {
	e.cy += y
	e.WrapCursorY()
}

func (e *Editor) SetRelativePosX(x int) {
	e.cx += x
	e.WrapCursorX()
}

func Save(e *Editor) {
	n, err := e.Save()
	if err != nil {
		if err == ErrPromptCanceled {
			e.SetStatusMessage("Save aborted")
		} else {
			e.SetStatusMessage("Can't save! I/O error: %s", err.Error())
		}
	} else {
		e.SetStatusMessage("%d bytes written to disk", n)
	}
}

func (e *Editor) SetMode(m EditorMode) {
	e.Mode = m
}
