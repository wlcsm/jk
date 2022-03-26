package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

var (
	stdinfd  = int(os.Stdin.Fd())
	stdoutfd = int(os.Stdout.Fd())
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

	// file content
	rows []*Row

	// whether or not the file has been modified
	modified bool

	filename string

	// status message and time the message was set
	statusmsg     string
	statusmsgTime time.Time

	cfg DisplayConfig

	// specify which syntax highlight to use.
	syntax *EditorSyntax

	// original termios: used to restore the state on exit.
	origTermios *unix.Termios
}

type DisplayConfig struct {
	Tabstop int
}

var defaultDisplayConfig = DisplayConfig{
	Tabstop: 8,
}

func (e *Editor) Init() error {
	termios, err := enableRawMode()
	if err != nil {
		return err
	}

	e.origTermios = termios
	ws, err := unix.IoctlGetWinsize(stdoutfd, unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		// fallback: get window size by moving the cursor to bottom-right
		// and getting the cursor position.
		if _, err = os.Stdout.Write([]byte("\x1b[999C\x1b[999B")); err != nil {
			return err
		}

		row, col, err := getCursorPosition()
		if err != nil {
			return err
		}

		e.screenRows = row
		e.screenCols = col

		return nil
	}

	e.screenRows = int(ws.Row) - 2 // make room for status-bar and message-bar
	e.screenCols = int(ws.Col)

	e.cfg = defaultDisplayConfig
	e.Mode = CommandMode
	return nil
}

type Key int32

// Assign an arbitrary large number to the following special keys
// to avoid conflicts with the normal keys.
const (
	keyEnter     Key = 10
	keyBackspace Key = 127
	keyEscape    Key = '\x1b'

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
	// Index within the file.
	idx int
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

func die(err error) {
	os.Stdout.WriteString("\x1b[2J") // clear the screen
	os.Stdout.WriteString("\x1b[H")  // reposition the cursor
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
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
		handled, err := keymap.Handler(e, k)
		if err != nil {
			return err
		}

		if handled {
			break
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
		if len(e.rows) == 0 && y == e.screenRows/3 {
			e.displayWelcomeMessage(w)
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
			color := SyntaxToColor(hl[i])
			if color != currentColor {
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

func (e *Editor) drawMessageBar(b *strings.Builder) {
	b.Write([]byte("\x1b[K"))
	msg := e.statusmsg
	if runewidth.StringWidth(msg) > e.screenCols {
		msg = runewidth.Truncate(msg, e.screenCols, "...")
	}
	// show the message if it's less than 5s old.
	if time.Since(e.statusmsgTime) < 5*time.Second {
		b.WriteString(msg)
	}
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

func (e *Editor) SetStatusMessage(format string, a ...interface{}) {
	e.statusmsg = fmt.Sprintf(format, a...)
	e.statusmsgTime = time.Now()
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

func (e *Editor) Save() (int, error) {
	return 0, nil
	//	// TODO: write to a new temp file, and then rename that file to the
	//	// actual file the user wants to overwrite, checking errors through
	//	// the whole process.
	//	if len(e.filename) == 0 {
	//		err := e.Prompt("Save as: %s (ESC to cancel)", nil)
	//		if err != nil {
	//			return 0, err
	//		}
	//		if cancelled {
	//			return 0, ErrPromptCanceled
	//		}
	//
	//		e.filename = fname
	//	}
	//
	//	f, err := os.OpenFile(e.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	//	if err != nil {
	//		return 0, err
	//	}
	//	defer f.Close()
	//	n, err := f.WriteString(e.rowsToString())
	//	if err != nil {
	//		return 0, err
	//	}
	//
	//	e.modified = false
	//	return n, nil
}

func (e *Editor) rowsToString() string {
	var b strings.Builder
	for _, row := range e.rows {
		b.WriteString(string(row.chars))
		b.WriteRune('\n')
	}
	return b.String()
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
				for _, row := range e.rows {
					e.updateHighlight(row)
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
	if err != nil {
		return err
	}
	defer f.Close()

	e.rows = make([]*Row, 0)

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Bytes()
		// strip off newline or cariage return
		bytes.TrimRightFunc(line, func(r rune) bool { return r == '\n' || r == '\r' })
		e.InsertRow(len(e.rows), string(line))
	}

	if err := s.Err(); err != nil {
		return err
	}

	e.modified = false
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
		e.updateRow(row)
	}
	e.cy++
	e.cx = 0
}

func (e *Editor) updateRow(row *Row) {
	var b strings.Builder
	col := 0
	for _, r := range row.chars {
		if r != '\t' {
			b.WriteRune(r)
			continue
		}

		// each tab must advance the cursor forward at least one column
		b.WriteRune(' ')
		col++
		// append spaces until we get to a tab stop
		for col%e.cfg.Tabstop != 0 {
			b.WriteRune(' ')
			col++
		}
	}
	row.render = b.String()
	e.updateHighlight(row)
}

func isSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.IndexRune(",.()+-/*=~%<>[]{}:;", r) != -1
}

func (e *Editor) updateHighlight(row *Row) {
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
	inComment := row.idx > 0 && e.rows[row.idx-1].hasUnclosedComment

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

		if kw, hl := e.checkIfKeyword(runes[idx:]); kw != "" {
			end := idx + len(kw)
			for idx < end {
				row.hl[idx] = hl
				idx++
			}
		}

		prevSep = isSeparator(r)
		idx++
	}

	changed := row.hasUnclosedComment != inComment
	row.hasUnclosedComment = inComment
	if changed && row.idx+1 < len(e.rows) {
		e.updateHighlight(e.rows[row.idx+1])
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
	f, err := enableLogs()
	if err != nil {
		die(err)
	}
	defer f.Close()

	var editor Editor

	if err := editor.Init(); err != nil {
		die(err)
	}
	defer editor.Close()

	if len(os.Args) > 1 {
		err := editor.OpenFile(os.Args[1])
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			die(err)
		}
	}

	editor.SetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit | Ctrl-F = find")

	for {
		editor.Render()
		if err := editor.ProcessKey(); err != nil {
			if err == ErrQuitEditor {
				break
			}
			die(err)
		}
	}
}

var LogFile = "mini.log"

func enableLogs() (*os.File, error) {
	f, err := os.OpenFile(LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return nil, errors.Wrapf(err, "opening file. filename=%s", LogFile)
	}

	log.SetOutput(f)
	log.Println("Logging begin")

	return f, nil
}

func (e *Editor) Close() error {
	if e.origTermios == nil {
		return fmt.Errorf("raw mode is not enabled")
	}

	// restore original termios.
	return unix.IoctlSetTermios(stdinfd, ioctlWriteTermios, e.origTermios)
}

func enableRawMode() (*unix.Termios, error) {
	t, err := unix.IoctlGetTermios(stdinfd, ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	raw := *t // make a copy to avoid mutating the original
	raw.Iflag &^= unix.BRKINT | unix.INPCK | unix.ISTRIP | unix.IXON
	// FIXME: figure out why this is not needed
	// termios.Oflag &^= unix.OPOST
	raw.Cflag |= unix.CS8
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1
	if err := unix.IoctlSetTermios(stdinfd, ioctlWriteTermios, &raw); err != nil {
		return nil, err
	}
	return t, nil
}
