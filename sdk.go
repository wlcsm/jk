package main

type SDK interface {
	InsertChar(c rune)
	DeleteChar()
	DeleteRow(at int)
	Find() error

	Prompt(prompt string, cb func(query string, k Key)) (string, bool, error)

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

// Prompt shows the given prompt in the status bar and get user input
// until to user presses the Enter key to confirm the input or until the user
// presses the Escape key to cancel the input. Returns the user input and nil
// if the user enters the input. Returns an empty string and ErrPromptCancel
// if the user cancels the input.
// It takes an optional callback function, which takes the query string and
// the last key pressed.
// Returns the full string when entered, whether it was canceled, and an error
func (e *Editor) Prompt(prompt string, cb func(query []rune, k Key)) (string, bool, error) {
	var text []rune
	var cancelled bool
loop:
	for {
		e.SetStatusMessage(prompt, string(text))
		e.Render()

		k, err := readKey()
		if err != nil {
			return "", false, err
		}

		switch k {
		case keyDelete, keyBackspace, Key(ctrl('h')):
			if len(text) != 0 {
				text = text[:len(text)-1]
			}
		case keyEscape:
			cancelled = true
			break loop
		case keyEnter:
			break loop
		default:

			if isPrintable(k) {
				text = append(text, rune(k))
			}
		}

		if cb != nil {
			cb(text, k)
		}
	}

	e.SetStatusMessage("")
	return string(text), cancelled, nil
}

/*** find ***/

func (e *Editor) Find() error {
	savedCx := e.cx
	savedCy := e.cy
	savedColOffset := e.colOffset
	savedRowOffset := e.rowOffset

//	var savedHl []SyntaxHL

	onKeyPress := func(query []rune, k Key) {
		// search for query and set e.cy, e.cx, e.rowOffset values.
		for i, row := range e.rows[e.cy:] {

			index := findSubstring(row.chars, query)
			if index == -1 {
				continue
			}

			// match found
			e.cy = i
			e.cx = index

			// TODO feel like there is a better way to do this, should decouple this behaviour
			// set rowOffset to bottom so that the next scroll() will scroll
			// upwards and the matching line will be at the top of the screen
			e.rowOffset = len(e.rows)
//
//			// highlight the matched string
//			savedHl = make([]SyntaxHL, len(row.hl))
//			copy(savedHl, row.hl)
//			for i := range query {
//				row.hl[rx+i] = hlMatch
//			}

			return
		}
	}

	_, canceled, err := e.Prompt("Search: %s", onKeyPress)

	// Get rid of the search highlight
	e.updateRow(e.rows[e.cy])

	// restore cursor position when the user cancels search
	if canceled {
		e.cx = savedCx
		e.cy = savedCy
		e.colOffset = savedColOffset
		e.rowOffset = savedRowOffset
	}

	return err
}

func findSubstring(text, query []rune) int {
outer:
	for i := range text {
		for j, q := range query {
			if text[i+j] != q {
				continue outer
			}
		}

		return i
	}

	return -1
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
