package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
)

type SDK interface {
	InsertChars(y, x int, c ...rune)
	DeleteRow(at int)
	FindInteractive()
	Find(x, y int, query []rune) (x1, y1 int)
	FindBack(x, y int, query []rune) (x1, y1 int)

	Row(y int) []rune
	SetRow(y int, chars []rune)
	NumRows() int

	LastSearch() []rune

	Word() int
	BackWord() int

	IsModified() bool

	ErrChan() chan<- error
	OpenFile(f string) error
	Prompt(prompt string, cb func(Key) (string, bool))
	StaticPrompt(prompt string, end func(string) error, cmpl CompletionFunc)
	Save() error
	SetMessage(format string, args ...interface{})
	Filename() string

	Delete(y, x1, x2 int)

	// Set the absolute position of the cursor's y (wrapped)
	SetY(y int)
	// Set the absolute position of the cursor's x (wrapped)
	SetX(x int)

	// Set the absolute position of the cursor's x (wrapped)
	WrapCursorX()
	// Wrap the cursor to keep it in bounds
	WrapCursorY()

	SetMode(m EditorMode)

	InsertRow(at int, chars []rune)

	X() int
	Y() int

	Cols() int
	Rows() int

	CenterCursor()

	ScreenBottom() int
	ScreenTop() int
	ScreenLeft() int
	ScreenRight() int
}

func (e *Editor) Row(y int) []rune {
	return e.rows[y].chars
}

func (e *Editor) NumRows() int {
	return len(e.rows)
}

type CompletionFunc func(a string) ([]CmplItem, error)

type CmplItem struct {
	Display string
	Real    string
}

func FileCompletion(a string) ([]CmplItem, error) {
	// Yes this will break on windows, idc
	i := strings.LastIndex(a, "/")
	if i == -1 {
		i = 0
	} else {
		i++
	}

	fileBasename := a[:i]
	fileHead := a[i:]

	log.Printf("fileBase: %s", fileBasename)

	files, err := os.ReadDir("./" + fileBasename)
	if err != nil {
		return nil, err
	}

	var res []CmplItem
	for _, f := range files {
		log.Printf("fil: %s", f.Name())
		if !strings.HasPrefix(f.Name(), fileHead) {
			continue
		}

		if f.IsDir() {
			res = append(res, CmplItem{
				Display: f.Name() + "/",
				Real:    fileBasename + f.Name() + "/",
			})
		} else if f.Type().IsRegular() {
			res = append(res, CmplItem{
				Display: f.Name(),
				Real:    fileBasename + f.Name(),
			})
		}
	}

	return res, nil
}

func Find(s []rune, f func(rune) bool) int {
	for i := range s {
		if f(s[i]) {
			return i
		}
	}

	return -1
}

func FindLeft(s []rune, f func(rune) bool) int {
	for i := len(s) - 1; i >= 0; i-- {
		if f(s[i]) {
			return i
		}
	}

	return -1
}

func (e *Editor) Word() int {
	x, y := e.X(), e.Y()
	row := e.rows[y].chars

	i := Find(row[x:], unicode.IsSpace)
	if i == -1 {
		return len(row)
	}

	j := Find(row[x+i:], func(r rune) bool { return !unicode.IsSpace(r) })
	if j == -1 {
		return len(row)
	}

	return x + i + j
}

func (e *Editor) BackWord() int {
	x, y := e.X(), e.Y()
	chars := e.rows[y].chars

	i := FindLeft(chars[:x], unicode.IsSpace)
	if i == -1 {
		return 0
	}

	// If the cursor is already at the beginning of the word, go to
	// the beginning of the next word
	if i == x-1 {
		i = FindLeft(chars[:x], func(r rune) bool { return !unicode.IsSpace(r) })
		if i == -1 {
			return 0
		}

		i = FindLeft(chars[:i], unicode.IsSpace)
		if i == -1 {
			return 0
		}
	}

	return i + 1
}

func (e *Editor) CenterCursor() {
	e.rowOffset = e.cy - (e.screenRows / 2)
	if e.rowOffset < 0 {
		e.rowOffset = 0
	}
}

func (e *Editor) Rows() int {
	return e.screenRows
}

func (e *Editor) Cols() int {
	return e.screenCols
}

func (e *Editor) Filename() string {
	return e.filename
}

func (e *Editor) IsModified() bool {
	return e.modified
}

func (e *Editor) X() int {
	return e.cx
}

func (e *Editor) Y() int {
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
}

func (e *Editor) InsertChars(y, x int, chars ...rune) {
	if e.cy == len(e.rows) {
		e.InsertRow(len(e.rows), []rune(""))
	}

	row := e.rows[e.cy]

	// make some room for the new chars
	row.chars = append(row.chars, make([]rune, len(chars))...)

	// shift the existing data back, and insert the chars in between
	copy(row.chars[x+len(chars):], row.chars[x:])
	copy(row.chars[x:], chars)

	e.updateRow(e.cy)
}

func (e *Editor) DeleteRow(at int) {
	e.rows = append(e.rows[:at], e.rows[at+1:]...)
}

// Prompt shows the given prompt in the status bar and get user input
//
// The mandatory callback is called with the user's input and returns the full
// text to display and a boolean indicating whether the promt should finish
func (e *Editor) Prompt(prompt string, cb func(k Key) (string, bool)) {
	if cb == nil {
		e.ErrChan() <- fmt.Errorf("can't give a nil function to Prompt")
		return
	}

	backup := Keymapping
	SetKeymapping([]KeyMap{{
		Name: PromptModeName,
		Handler: func(e SDK, k Key) (bool, error) {
			s, finished := cb(k)

			// Restore the previous keymapping when finished
			if finished {
				SetKeymapping(backup)
				return true, nil
			}

			e.SetMessage(prompt + s)
			return false, nil
		},
	}})

	e.SetMode(PromptMode)
	e.SetMessage(prompt)
}

func (e *Editor) LastSearch() []rune {
	return e.lastSearch
}

func (e *Editor) FindInteractive() {
	savedCx := e.cx
	savedCy := e.cy
	savedColOffset := e.colOffset
	savedRowOffset := e.rowOffset

	var query []rune

	onKeyPress := func(k Key) (string, bool) {
		switch k {
		case keyDelete, keyBackspace:
			if len(query) != 0 {
				query = query[:len(query)-1]

				// This forces the editor to search again to
				// see if the current word is indeed the
				// closest match. Yes making a stack containing
				// the previous matches would be better, but it
				// is somewhat unecessary at the moment
				e.cx = savedCx
				e.cy = savedCy

			}
		case keyEscape, Key(ctrl('q')):
			// restore cursor position when the user cancels search
			e.cx = savedCx
			e.cy = savedCy
			e.colOffset = savedColOffset
			e.rowOffset = savedRowOffset

			e.SetMessage("")

			return "", true
		case keyEnter, keyCarriageReturn:
			e.SetMessage("")
			e.lastSearch = query

			return "", true
		default:
			if isPrintable(k) {
				query = append(query, rune(k))
			}
		}

		x, y := e.Find(e.cx, e.cy, query)
		if x == -1 {
			e.cx = savedCx
			e.cy = savedCy
			e.colOffset = savedColOffset
			e.rowOffset = savedRowOffset

			return string(query), false
		}

		// Set cursor to beginning of match
		e.cy = y
		e.cx = x

		// Try to make the text in the middle of the screen
		e.SetRowOffset(e.cy - e.screenRows/2)

		return string(query), false
	}

	e.Prompt("Search: ", onKeyPress)
}

func (e *Editor) Find(x1, y1 int, query []rune) (x, y int) {
	x = findSubstring(e.rows[y1].chars[x1:], query)
	if x != -1 {
		return x1 + x, y1
	}

	// The real search
	for y = y1 + 1; y < len(e.rows); y++ {
		if x = findSubstring(e.rows[y].chars, query); x != -1 {
			return x, y
		}
	}

	return -1, -1
}

func (e *Editor) FindBack(x1, y1 int, query []rune) (x, y int) {
	x = findSubstringBack(e.rows[y1].chars, query, x1)
	if x != -1 {
		return x, y1
	}

	// The real search
	for y = y1 - 1; y >= 0; y-- {
		if x = findSubstringBack(e.rows[y].chars, query, len(e.rows[y].chars)); x != -1 {
			return x, y
		}
	}

	return -1, -1
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
	if len(text) < len(query) {
		return -1
	}

outer:
	for i := range text[:len(text)-len(query)+1] {
		for j := range query {
			if text[i+j] != query[j] {
				continue outer
			}
		}

		return i
	}

	return -1
}

func findSubstringBack(text, query []rune, offset int) int {
	if len(text) < len(query) {
		return -1
	}

	log.Printf("query: %s, len: %d", string(query), len(query))
	// Make sure text[i+j] doesn't overflow
	if offset > len(text)-len(query) {
		offset = len(text) - len(query)
	}

outer:
	for i := offset; i >= 0; i-- {
		log.Printf("text: %s, i: %d", string(text[i:i+len(query)]), i)
		for j := range query {
			if text[i+j] != query[j] {
				continue outer
			}
		}

		return i
	}

	return -1
}

func (e *Editor) SetRow(at int, chars []rune) {
	e.rows[at].chars = chars

	e.updateRow(at)
}

func (e *Editor) InsertRow(at int, chars []rune) {
	row := Row{chars: chars}
	if at > 0 {
		row.hasUnclosedComment = e.rows[at-1].hasUnclosedComment
	}

	// grow the buffer
	e.rows = append(e.rows, &Row{})
	copy(e.rows[at+1:], e.rows[at:])
	e.rows[at] = &row

	e.updateRow(at)
}

func (e *Editor) Delete(y, x1, x2 int) {
	log.Printf("y: %d, x1: %d, x2: %d", y, x1, x2)
	row := e.rows[y].chars
	e.rows[y].chars = append(row[:x1], row[x2+1:]...)
	log.Printf("row: %s", string(e.rows[y].chars))
	e.updateRow(y)
}

func (e *Editor) SetY(y int) {
	e.cy = y
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

func (e *Editor) SetX(x int) {
	e.cx = x
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

func (e *Editor) ErrChan() chan<- error {
	return e.errChan
}

// StaticPrompt is a "normal" prompt designed to only get input from the user.
// It you want things to happen when you press any key, then use Prompt
func (e *Editor) StaticPrompt(prompt string, end func(string) error, comp CompletionFunc) {
	var input string

	e.Prompt(prompt, func(k Key) (string, bool) {
		log.Printf("key is: %s", string(k))

		switch k {
		case keyEnter, keyCarriageReturn:
			if err := end(input); err != nil {
				e.ErrChan() <- err
			}

			return input, true
		case keyEscape, Key(ctrl('q')):
			return "", true
		case keyBackspace, keyDelete:
			if len(input) > 0 {
				input = input[:len(input)-1]
			}
		case Key('\t'):
			if comp == nil {
				break
			}

			opts, err := comp(input)
			if err != nil {
				break
			}
			log.Printf("completion options: %v", opts)

			if len(opts) == 1 {
				input = opts[0].Real
			}
		default:
			if isPrintable(k) {
				input += string(k)
			}
		}

		return input, false
	})
}
