package main

import (
	"fmt"
	"log"
)

type SDK interface {
	InsertChar(c rune)
	DeleteChar()
	DeleteRow(at int)
	Find() error
	BackWord() int
	ForwardWord()
	InsertNewline()
	IsModified() bool

	OpenFile(f string) error
	Prompt(prompt string, cb func(k Key) (string, bool)) error
	StaticPrompt(prompt string) (string, error)
	Save() error
	SetMessage(format string, args ...interface{})
	Filename() string

	// Delete until the rune
	DeleteUntil(i int)
	// Search to the left until the predicate matches
	FindLeft(x, y int, f func(s rune) bool) (int, bool)
	// Search to the left until the predicate matches
	FindRight(func(r rune) bool) (int, bool)

	// Set the absolute position of the cursor's y (wrapped)
	SetPosY(y int)
	SetPosMaxY()
	// Set the absolute position of the cursor's x (wrapped)
	SetPosX(x int)
	SetPosMaxX()

	// Set the absolute position of the cursor's x (wrapped)
	WrapCursorX()
	// Wrap the cursor to keep it in bounds
	WrapCursorY()

	SetMode(m EditorMode)

	SetRow(at int, chars string)
	InsertRow(at int, chars string)

	CX() int
	CY() int

	ScreenBottom() int
	ScreenTop() int
	ScreenLeft() int
	ScreenRight() int
}

func (e *Editor) Filename() string {
	return e.filename
}

func (e *Editor) IsModified() bool {
	return e.modified
}

func (e *Editor) CX() int {
	return e.cx
}

func (e *Editor) CY() int {
	return e.cy
}

func (e *Editor) ScreenBottom() int {
	return e.rowOffset + e.screenRows - 1
}

func (e *Editor) ScreenTop() int {
	return e.rowOffset + 1
}

func (e *Editor) ScreenLeft() int {
	return e.colOffset + 1
}

func (e *Editor) ScreenRight() int {
	return e.colOffset + e.screenCols + 1
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
	e.updateRow(e.cy)
	e.cx++
	e.modified = true
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
		e.updateRow(e.cy)
		e.cx--
		e.modified = true
	} else {
		prevRow := e.rows[e.cy-1]
		e.cx = len(prevRow.chars)
		prevRow.appendChars(row.chars)
		e.DeleteRow(e.cy)
		e.updateHighlight(e.cy)
		e.cy--
	}
}

func (e *Editor) DeleteRow(at int) {
	if at < 0 || len(e.rows) <= at {
		return
	}

	e.rows = append(e.rows[:at], e.rows[at+1:]...)

	e.modified = true
	e.WrapCursorY()
	e.WrapCursorX()
}

// Prompt shows the given prompt in the status bar and get user input
//
// The mandatory callback is called with the user's input and returns the full
// text to display and a boolean indicating whether the promt should finish
func (e *Editor) Prompt(prompt string, cb func(k Key) (string, bool)) error {
	if cb == nil {
		return fmt.Errorf("Can't give a nil function to Prompt")
	}

	var finished bool

	for !finished {
		e.SetMessage(prompt)
		e.Render()

		k, err := readKey()
		if err != nil {
			return err
		}

		prompt, finished = cb(k)
	}

	e.SetMessage("")
	return nil
}

/*** find ***/

func (e *Editor) Find() error {
	savedCx := e.cx
	savedCy := e.cy
	savedColOffset := e.colOffset
	savedRowOffset := e.rowOffset

	var (
		query []rune
		found bool
	)

	onKeyPress := func(k Key) (string, bool) {
		switch k {
		case keyDelete, keyBackspace:
			if len(query) != 0 {
				query = query[:len(query)-1]
			}
		case keyEscape:
			return "", true
		case keyEnter, keyCarriageReturn:
			found = true
			return "", true
		default:
			if isPrintable(k) {
				query = append(query, rune(k))
			}
		}

		// search for query and set e.cy, e.cx, e.rowOffset values.
		for i, row := range e.rows[e.cy:] {
			index := findSubstring(row.chars, query)
			if index == -1 {
				continue
			}

			// match found
			e.cy += i
			e.cx = index

			// Try to make the text in the middle of the screen
			e.SetRowOffset(e.cy - e.screenRows/2)

			// highlight the matched string
			savedHl := make([]SyntaxHL, len(row.hl))
			copy(savedHl, row.hl)
			for i := range query {
				row.hl[index+i] = hlMatch
			}

			break
		}

		return "Search: " + string(query), false
	}

	err := e.Prompt("Search: ", onKeyPress)

	// Get rid of the search highlight
	e.updateRow(e.cy)

	// restore cursor position when the user cancels search
	if !found {
		e.cx = savedCx
		e.cy = savedCy
		e.colOffset = savedColOffset
		e.rowOffset = savedRowOffset
	}

	return err
}

func (e *Editor) SetRowOffset(y int) {
	if y < 0 {
		y = 0
	}

	e.rowOffset = y
}

func (e *Editor) SetColOffset(x int) {
	if x < 0 {
		x = 0
	}

	e.colOffset = x
}

// return the place where the substring starts
func findSubstring(text, query []rune) int {
outer:
	for i := range text {
		for j := range query {
			if text[i+j] != query[j] {
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
	e.updateRow(at)

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
	if at > 0 {
		row.hasUnclosedComment = e.rows[at-1].hasUnclosedComment
	}

	// grow the buffer
	e.rows = append(e.rows, &Row{})
	copy(e.rows[at+1:], e.rows[at:])
	e.rows[at] = row

	e.updateRow(at)

	// adjust the cursor
	if at <= e.cy {
		e.cy++
	}
}

func (e *Editor) DeleteUntil(x int) {
	row := e.rows[e.cy]
	row.chars = append(row.chars[:x], row.chars[e.cx:]...)
	e.cx = x

	e.updateRow(e.cy)
}

func (e *Editor) FindLeft(x, y int, f func(s rune) bool) (int, bool) {
	row := e.rows[y]

	// Find the first instance of the rune starting at the cursor
	for i := x - 1; i >= 0; i-- {
		if f(row.chars[i]) {
			return i, true
		}
	}

	return 0, false
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

func (e *Editor) SetMode(m EditorMode) {
	e.Mode = m

	if m == InsertMode {
		for i, keymap := range Keymapping {
			if keymap.Name == CommandModeName {
				Keymapping[i] = InsertModeMap
				return
			}
		}
	} else {
		for i, keymap := range Keymapping {
			if keymap.Name == InsertModeName {
				Keymapping[i] = CommandModeMap
				return
			}
		}
	}
}

// StaticPrompt is a "normal" prompt designed to only get input from the user.
// It you want things to happen when you press any key, then use Prompt
func (e *Editor) StaticPrompt(prompt string) (input string, err error) {
	canceled := false
	err = e.Prompt(prompt, func(k Key) (string, bool) {
		log.Printf("key is: %s", string(k))
		switch k {
		case keyEnter, keyCarriageReturn:
			return "", true
		case keyEscape, Key(ctrl('q')):
			canceled = true
			return "", true
		case keyBackspace, keyDelete:
			if len(input) > 0 {
				input = input[:len(input)-1]
			}
		default:
			if isPrintable(k) {
				input += string(k)
			}
		}

		return prompt + input, false
	})
	e.SetMessage("")

	if err != nil {
		return "", err
	}

	if canceled {
		return input, ErrPromptCanceled
	}

	return input, nil
}
