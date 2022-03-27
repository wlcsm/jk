package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/pkg/errors"
	"golang.org/x/term"
)

var ErrQuitEditor = errors.New("quit editor")

type EditorMode int8

const (
	InsertMode EditorMode = iota + 1
	CommandMode
)

type Editor struct {
	Mode EditorMode

	errChan chan error

	// cursor coordinates
	cx, cy int // cx is an index into Row.chars
	rx     int // rx is an index into []rune(Row.render)

	// offsets. Offset is calculated in the number of runes
	rowOffset int
	colOffset int

	// screen size
	screenRows int
	screenCols int

	showWelcomeScreen bool

	// file content
	rows []*Row

	// whether or not the file has been modified
	modified bool

	filename string

	// status message and time the message was set
	statusmsg string

	// General settings like tabstop
	cfg DisplayConfig

	// specify which syntax highlight to use.
	syntax *EditorSyntax
}

type DisplayConfig struct {
	Tabstop int
}

var defaultDisplayConfig = DisplayConfig{
	Tabstop: 8,
}

func (e *Editor) Init() error {
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}

	// make room for status-bar and message-bar
	e.screenRows = rows - 2
	e.screenCols = cols

	e.cfg = defaultDisplayConfig
	e.Mode = CommandMode
	return nil
}

type Key int32

// Assign an arbitrary large number to the following special keys
// to avoid conflicts with the normal keys.
const (
	keyEnter          Key = 10
	keyCarriageReturn Key = 13
	keyBackspace      Key = 127
	keyEscape         Key = '\x1b'

	keyArrowLeft Key = iota + 1000
	keyArrowRight
	keyArrowUp
	keyArrowDown
	keyDelete
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
)

type Row struct {
	// Raw character data for the row as an array of runes.
	chars []rune
	// Actual chracters to draw on the screen.
	render string
	// Syntax highlight value for each rune in the render string.
	hl []SyntaxHL
	// Indicates whether this row has unclosed multiline comment.
	hasUnclosedComment bool
}

// ctrl returns a byte resulting from pressing the given ASCII character with the ctrl-key.
func ctrl(char byte) byte {
	return char & 0x1f
}

var escapeCodeToKey = map[string]Key{
	"\x1b[A":  keyArrowUp,
	"\x1b[B":  keyArrowDown,
	"\x1b[C":  keyArrowRight,
	"\x1b[D":  keyArrowLeft,
	"\x1b[1~": keyHome,
	"\x1b[7~": keyHome,
	"\x1b[H":  keyHome,
	"\x1bOH":  keyHome,
	"\x1b[4~": keyEnd,
	"\x1b[8~": keyEnd,
	"\x1b[F":  keyEnd,
	"\x1bOF":  keyEnd,
	"\x1b[3~": keyDelete,
	"\x1b[5~": keyPageUp,
	"\x1b[6~": keyPageDown,
}

// readKey reads a key press input from stdin.
func readKey() (Key, error) {
	buf := make([]byte, 4)
	for {
		log.Printf("surprise log")
		n, err := os.Stdin.Read(buf)
		if err != nil && err != io.EOF {
			return 0, err
		}

		if n == 0 {
			continue
		}

		buf = bytes.TrimRightFunc(buf, func(r rune) bool { return r == 0 })
		key, ok := escapeCodeToKey[string(buf)]
		if !ok {
			return Key(buf[0]), nil
		}

		return key, nil
	}
}

type Direction int8

const (
	DirectionUp Direction = iota + 1
	DirectionDown
	DirectionLeft
	DirectionRight
)

func RepositionCursor() {
	os.Stdout.WriteString(RepositionCursorCode)
}

func ClearScreen() {
	os.Stdout.WriteString(ClearScreenCode)
}

type EscapeCodes string

const (
	RepositionCursorCode = "\x1b[H"
	ResetColorCode       = "\x1b[39m"
	ClearLineCode        = "\x1b[K"
	ClearScreenCode      = "\x1b[2J"
)

// ProcessKey processes a key read from stdin.
// Returns errQuitEditor when user requests to quit.
func (e *Editor) ProcessKey() error {
	k, err := readKey()
	if err != nil {
		return err
	}

	for _, keymap := range Keymapping {
		log.Printf("just pressed: %s", string(k))

		handled, err := keymap.Handler(e, k)
		if err != nil {
			return err
		}

		if handled {
			return nil
		}
	}

	return nil
}

func (e *Editor) displayWelcomeMessage(w io.Writer) {
	welcomeMsg := fmt.Sprintf("Mini editor -- version %s", Version)
	if runewidth.StringWidth(welcomeMsg) > e.screenCols {
		welcomeMsg = utf8Slice(welcomeMsg, 0, e.screenCols)
	}
	padding := (e.screenCols - runewidth.StringWidth(welcomeMsg)) / 2
	if padding > 0 {
		w.Write([]byte("~"))
		padding--
	}
	for ; padding > 0; padding-- {
		w.Write([]byte(" "))
	}

	w.Write([]byte(welcomeMsg))
}

func (e *Editor) drawRows(w io.Writer) {
	for y := 0; y < e.screenRows; y++ {
		e.drawRow(w, y)

		w.Write([]byte(ClearLineCode))
		w.Write([]byte("\r\n"))
	}
}

func (e *Editor) drawRow(w io.Writer, y int) {
	filerow := y + e.rowOffset
	if filerow >= len(e.rows) {
		// The display message should not be here, you should not be
		// able to get back to it once passed
		if e.showWelcomeScreen && len(e.rows) == 0 && y == e.screenRows/3 {
			e.displayWelcomeMessage(w)
			e.showWelcomeScreen = false
		} else {
			w.Write([]byte("~"))
		}

		return
	}

	var (
		line string
		hl   []SyntaxHL
	)

	// Use the offset to remove the first part of the render string
	row := e.rows[filerow]
	if runewidth.StringWidth(row.render) > e.colOffset {
		line = utf8Slice(row.render, e.colOffset, utf8.RuneCountInString(row.render))
		hl = e.rows[filerow].hl[e.colOffset:]
	}

	// Use the number of columns to truncate the end
	if runewidth.StringWidth(line) > e.screenCols {
		line = runewidth.Truncate(line, e.screenCols, "")
		hl = hl[:utf8.RuneCountInString(line)]
	}

	currentColor := -1 // keep track of color to detect color change
	for i, r := range line {
		if unicode.IsControl(r) {
			// deal with non-printable characters (e.g. Ctrl-A)
			sym := '?'
			if r < 26 {
				sym = '@' + r
			}

			setColor(w, InvertedColor)
			w.Write(rToB(sym))
			clearFormatting(w)

			// restore the current color
			if currentColor != -1 {
				setColor(w, currentColor)
			}
		} else {
			if color := SyntaxToColor(hl[i]); color != currentColor {
				currentColor = color
				setColor(w, color)
			}

			w.Write(rToB(r))
		}
	}

	setColor(w, ClearColor)
}

const (
	ClearColor    = 39
	InvertedColor = 7
)

func setColor(b io.Writer, c int) {
	b.Write([]byte("\x1b[" + strconv.Itoa(c) + "m"))
}

func clearFormatting(b io.Writer) {
	b.Write([]byte("\x1b[m"))
}

// utf8Slice slice the given string by utf8 character.
func utf8Slice(s string, start, end int) string {
	return string([]rune(s)[start:end])
}

var ClearFromCusorToEndOfLine = []byte("\x1b[K")

func (e *Editor) drawMessageBar(b *strings.Builder) {
	b.Write(ClearFromCusorToEndOfLine)
	msg := e.statusmsg
	if runewidth.StringWidth(msg) > e.screenCols {
		msg = runewidth.Truncate(msg, e.screenCols, "...")
	}

	b.Write([]byte(msg))
}

// Cursor position (which is calculated in runes) to the visual position
func (e *Editor) rowCxToRx(row *Row, cx int) int {
	rx := 0
	for _, r := range row.chars[:cx] {
		if r == '\t' {
			rx += (e.cfg.Tabstop) - (rx % e.cfg.Tabstop)
		} else {
			rx += runewidth.RuneWidth(r)
		}
	}
	return rx
}

func (e *Editor) rowRxToCx(row *Row, rx int) int {
	if len(row.chars) == 0 {
		return 0
	}

	curRx := 0
	for i, r := range row.chars {
		if r == '\t' {
			curRx += (e.cfg.Tabstop) - (curRx % e.cfg.Tabstop)
		} else {
			curRx += runewidth.RuneWidth(r)
		}

		if curRx > rx {
			return i
		}
	}
	panic(fmt.Sprintf("unreachable, row=%v, rx=%d", row, rx))
}

func (e *Editor) scroll() {
	e.rx = 0
	if e.cy < len(e.rows) {
		e.rx = e.rowCxToRx(e.rows[e.cy], e.cx)
	}
	// scroll up if the cursor is above the visible window.
	if e.cy < e.rowOffset {
		e.rowOffset = e.cy
	}
	// scroll down if the cursor is below the visible window.
	if e.cy >= e.rowOffset+e.screenRows {
		e.rowOffset = e.cy - e.screenRows + 1
	}
	// scroll left if the cursor is left of the visible window.
	if e.rx < e.colOffset {
		e.colOffset = e.rx
	}
	// scroll right if the cursor is right of the visible window.
	if e.rx >= e.colOffset+e.screenCols {
		e.colOffset = e.rx - e.screenCols + 1
	}
}

// Render refreshes the screen.
func (e *Editor) Render() {
	e.WrapCursorY()
	e.WrapCursorX()
	e.scroll()

	var b strings.Builder

	b.Write([]byte("\x1b[?25l")) // hide the cursor
	b.Write([]byte("\x1b[H"))    // reposition the cursor at the top left.

	e.drawRows(&b)
	e.drawStatusBar(&b)
	e.drawMessageBar(&b)

	// position the cursor
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH", (e.cy-e.rowOffset)+1, (e.rx-e.colOffset)+1))

	// show the cursor
	b.Write([]byte("\x1b[?25h"))
	os.Stdout.WriteString(b.String())
}

func (e *Editor) SetMessage(format string, a ...interface{}) {
	e.statusmsg = fmt.Sprintf(format, a...)
}

func getCursorPosition() (row, col int, err error) {
	if _, err = os.Stdout.Write([]byte("\x1b[6n")); err != nil {
		return
	}
	if _, err = fmt.Fscanf(os.Stdin, "\x1b[%d;%d", &row, &col); err != nil {
		return
	}
	return
}

var ErrPromptCanceled = fmt.Errorf("user canceled the input prompt")

func isPrintable(k Key) bool {
	return !unicode.IsControl(rune(k)) && unicode.IsPrint(rune(k)) && !isArrowKey(k)
}

func isArrowKey(k Key) bool {
	return k == keyArrowUp || k == keyArrowRight || k == keyArrowDown || k == keyArrowLeft
}

func (e *Editor) Save() error {
	if len(e.filename) == 0 {
		filename, err := e.StaticPrompt("Save as: %s (ESC to cancel)")
		if err != nil {
			return err
		}

		e.filename = filename
	}

	f, err := os.OpenFile(e.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, row := range e.rows {
		if _, err := f.Write([]byte(string(row.chars))); err != nil {
			return err
		}
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return err
		}
	}

	e.modified = false
	return nil
}

// Fairly basic version. Probably can make it faster *if need be*
func rToB(r rune) []byte {
	return []byte(string(r))
}

func (e *Editor) detectSyntax() {
	e.syntax = nil
	if len(e.filename) == 0 {
		return
	}

	ext := filepath.Ext(e.filename)

	for _, syntax := range HLDB {
		for _, pattern := range syntax.filematch {
			isExt := strings.HasPrefix(pattern, ".")
			if (isExt && pattern == ext) ||
				(!isExt && strings.Index(e.filename, pattern) != -1) {
				e.syntax = syntax
				for i := range e.rows {
					e.updateHighlight(i)
				}
				return
			}
		}
	}
}

// OpenFile opens a file with the given filename.
// If a file does not exist, it returns os.ErrNotExist.
func (e *Editor) OpenFile(filename string) error {
	e.filename = filename
	e.detectSyntax()

	f, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		f, err = os.Create(filename)
		e.modified = true
	} else {
		e.modified = false
	}

	if err != nil {
		return err
	}
	defer f.Close()

	e.rows = make([]*Row, 0)

	s := bufio.NewScanner(f)
	for i := 0; s.Scan(); i++ {
		line := s.Bytes()
		// strip off newline or cariage return
		bytes.TrimRightFunc(line, func(r rune) bool { return r == '\n' || r == '\r' })
		e.rows = append(e.rows, &Row{
			chars: []rune(string(line)),
		})

		e.updateRow(i)
	}

	if err := s.Err(); err != nil {
		return err
	}

	return nil
}

func (e *Editor) InsertNewline() {
	if e.cx == 0 {
		e.InsertRow(e.cy, "")
	} else {
		row := e.rows[e.cy]
		e.InsertRow(e.cy+1, string(row.chars[e.cx:]))
		// reassignment needed since the call to InsertRow
		// invalidates the pointer.
		row = e.rows[e.cy]
		row.chars = row.chars[:e.cx]
		e.updateRow(e.cy)
	}

	e.cy++
	e.cx = 0
}

func (e *Editor) updateRow(y int) {
	var b strings.Builder
	row := e.rows[y]
	cols := 0
	for _, r := range row.chars {
		if r != '\t' {
			b.WriteRune(r)
			cols += runewidth.RuneWidth(r)
			continue
		}

		// each tab must advance the cursor forward at least one column
		b.WriteRune(' ')
		cols++
		// append spaces until we get to a tab stop
		for cols%e.cfg.Tabstop != 0 {
			b.WriteRune(' ')
			cols++
		}

	}

	row.render = b.String()
	e.updateHighlight(y)
}

func isSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.IndexRune(",.()+-/*=~%<>[]{}:;", r) != -1
}

func (e *Editor) updateHighlight(y int) {
	row := e.rows[y]

	// TODO why can't we just use len(row.chars)? for some reason this panics
	row.hl = make([]SyntaxHL, utf8.RuneCountInString(row.render))
	for i := range row.hl {
		row.hl[i] = hlNormal
	}

	if e.syntax == nil {
		return
	}

	// whether the previous rune was a separator
	prevSep := true

	// zero when outside a string, set to the quote character ( ' or ")  in the string
	var strQuote rune

	// indicates whether we are inside a multi-line comment.
	inComment := y > 0 && e.rows[y-1].hasUnclosedComment

	idx := 0
	runes := []rune(row.render)
	for idx < len(runes) {
		r := runes[idx]
		prevHl := hlNormal
		if idx > 0 {
			prevHl = row.hl[idx-1]
		}

		// Single line comments
		if e.syntax.scs != "" && strQuote == 0 && !inComment {
			if strings.HasPrefix(string(runes[idx:]), e.syntax.scs) {
				for idx < len(runes) {
					row.hl[idx] = hlComment
					idx++
				}
				break
			}
		}

		// Multiline comments
		if e.syntax.mcs != "" && e.syntax.mce != "" && strQuote == 0 {
			if inComment {
				row.hl[idx] = hlMlComment
				if strings.HasPrefix(string(runes[idx:]), e.syntax.mce) {
					for j := 0; j < len(e.syntax.mce); j++ {
						row.hl[idx] = hlMlComment
						idx++
					}
					inComment = false
					prevSep = true
				} else {
					idx++
				}
				continue
			} else if strings.HasPrefix(string(runes[idx:]), e.syntax.mcs) {
				for j := 0; j < len(e.syntax.mcs); j++ {
					row.hl[idx] = hlMlComment
					idx++
				}
				inComment = true
				continue
			}
		}

		if e.syntax.highlightStrings {
			if strQuote != 0 {
				row.hl[idx] = hlString
				// deal with escape quote when inside a string
				if r == '\\' && idx+1 < len(runes) {
					row.hl[idx+1] = hlString
					idx += 2
					continue
				}

				if r == strQuote {
					strQuote = 0
				}

				idx++
				prevSep = true
				continue
			} else {
				if r == '"' || r == '\'' {
					strQuote = r
					row.hl[idx] = hlString
					idx++
					continue
				}
			}
		}

		if e.syntax.highlightNumbers {
			if unicode.IsDigit(r) && (prevSep || prevHl == hlNumber) ||
				r == '.' && prevHl == hlNumber {
				row.hl[idx] = hlNumber
				idx++
				prevSep = false
				continue
			}
		}

		if prevSep {
			if kw, hl := e.checkIfKeyword(runes[idx:]); kw != "" {
				end := idx + len(kw)
				for idx < end {
					row.hl[idx] = hl
					idx++
				}
			}
		}

		prevSep = isSeparator(r)
		idx++
	}

	changed := row.hasUnclosedComment != inComment
	row.hasUnclosedComment = inComment
	if changed && y+1 < len(e.rows) {
		e.updateHighlight(y + 1)
	}
}

func (e *Editor) checkIfKeyword(text []rune) (string, SyntaxHL) {
	kw := checkKeywordMatch(e.syntax.keywords, text)
	if len(kw) != 0 {
		return kw, hlKeyword1
	}

	kw = checkKeywordMatch(e.syntax.keywords2, text)
	if len(kw) != 0 {
		return kw, hlKeyword2
	}

	return "", 0
}

// Check if any of the keywords are a prefix of text, and also that it isn't
// just a substring of the a bigger word in text
func checkKeywordMatch(keywords []string, text []rune) string {
	for _, kw := range keywords {
		length := utf8.RuneCountInString(kw)
		if length > len(text) {
			continue
		}

		// check if we have a match
		if kw != string(text[:length]) {
			continue
		}

		// check that this is the entire word, either
		// there are no characters after this, or the
		// next character is a separator
		if length != len(text) && !isSeparator(text[length]) {
			continue
		}

		return kw
	}

	return ""
}

func main() {
	if ok := Run(); ok {
		os.Exit(2)
	}
}

type DisplaySettings struct {
	X         int `json:"x"`
	Y         int `json:"y"`
	RowOffset int `json:"row_offset"`
	ColOffset int `json:"col_offset"`
}

func Run() bool {
	var cfg DisplaySettings
	argIndex := 1
	if len(os.Args) == 3 {
		if os.Args[1] == "-z" {
			out, err := os.ReadFile(CacheFile)
			if err != nil {
				panic(err)
			}

			if err = json.Unmarshal(out, &cfg); err != nil {
				panic(err)
			}

			argIndex = 2
		}
	}

	restarted := false

	defer func() {
		if !restarted {
			os.Stdout.WriteString(ClearScreenCode)
			os.Stdout.WriteString(RepositionCursorCode)
			if err := recover(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %+v\n", err)
				fmt.Fprintf(os.Stderr, "stack: %s\n", debug.Stack())
				os.Exit(1)
			}
		}
	}()

	f, err := enableLogs()
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Set the terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

	defer term.Restore(int(os.Stdin.Fd()), oldState)

	var editor Editor
	if err := editor.Init(); err != nil {
		panic(err)
	}

	editor.cx = cfg.X
	editor.cy = cfg.Y
	editor.rowOffset = cfg.RowOffset
	editor.colOffset = cfg.ColOffset

	if len(os.Args) > 1 {
		err := editor.OpenFile(os.Args[argIndex])
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			panic(err)
		}
	}

	for {
		editor.Render()
		log.Println("hello")
		if err := editor.ProcessKey(); err != nil {
			if err == ErrQuitEditor {
				break
			}
			if err == RestartEditor {
				if err := editor.saveDisplay(); err != nil {
					panic(err)
				}
				if err := editor.rebuild(); err != nil {
					panic(err)
				}

				restarted = true
				return true
			}

			panic(err)
		}
	}

	return false
}

var RestartEditor = fmt.Errorf("yes")

func (e *Editor) rebuild() error {
	log.Println("befoere rebuild")
	cmd := exec.Command("make", "install")
	cmd.Dir = "/home/wlcsm/go/src/github.com/mini"

	log.Println("befoere rebuild")
	l, err := cmd.Output()
	log.Printf("rebuilding returned: %s", l)
	if err != nil {
		log.Printf("rebuilding error: %+v", err)
		return errors.Wrap(err, "here")
	}

	return nil
}

func (e *Editor) saveDisplay() error {
	out, err := json.Marshal(DisplaySettings{
		X:         e.cx,
		Y:         e.cy,
		RowOffset: e.rowOffset,
		ColOffset: e.colOffset,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(CacheFile, out, 0o644)
}

var (
	LogFile   = "/home/wlcsm/go/src/github.com/mini/mini.log"
	CacheFile = "/home/wlcsm/go/src/github.com/mini/cache.json"
)

func enableLogs() (*os.File, error) {
	f, err := os.OpenFile(LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file. filename=%s", LogFile)
	}

	log.SetOutput(f)
	log.Println("Logging begin")

	return f, nil
}
