package main

import (
	"fmt"
	"io"

	"github.com/mattn/go-runewidth"
)

const Version = "dev"

func (e *Editor) Keymapping(k Key) error {
	switch k {
	case keyEnter:
		e.InsertNewline()

	case Key(ctrl('q')):
		if e.modified {
			var quit bool

			e.Prompt("WARNING!!! File has unsaved changes. Press Ctrl-Q again to quit.",
				func(k Key) (string, bool) {
					if k == Key(ctrl('q')) {
						quit = true
					}

					return "", true
				})

			if !quit {
				return nil
			}
		}

		ClearScreen()
		RepositionCursor()
		return ErrQuitEditor

	case Key(ctrl('s')):
		e.Save()
	case Key(ctrl('f')):
		err := e.Find()
		if err != nil {
			if err == ErrPromptCanceled {
				e.SetStatusMessage("")
			} else {
				return err
			}
		}
	case keyBackspace:
		e.DeleteChar()
	case keyPageUp:
		e.SetPosY(e.ScreenTop())
	case keyPageDown:
		e.SetPosY(e.ScreenBottom())
	case keyArrowUp:
		e.SetRelativePosY(-1)
	case keyArrowDown:
		e.SetRelativePosY(1)
	case keyArrowLeft:
		e.SetRelativePosX(-1)
	case keyArrowRight:
		e.SetRelativePosX(1)
	case Key(ctrl('c')):
		e.SetMode(CommandMode)
	case Key(ctrl('W')):
		e.DeleteUntil(rune(' '))
	default:
		if e.Mode == CommandMode {
			e.CommandModeMapping(k)
		} else {
			e.InsertChar(rune(k))
		}
	}

	return nil
}

func (e *Editor) CommandModeMapping(k Key) error {

	switch k {
	case Key('j'):
		e.SetRelativePosY(1)
	case Key('k'):
		e.SetRelativePosY(-1)
	case Key('h'):
		e.SetRelativePosX(-1)
	case Key('l'):
		e.SetRelativePosX(1)
	case Key('i'):
		e.SetMode(InsertMode)
	case Key('o'):
		e.InsertRow(e.CY()+1, "")
		e.SetRelativePosY(1)
		e.SetMode(InsertMode)
	case Key('0'):
		e.SetPosX(0)
	case Key('$'):
		e.SetPosMaxX()
	case Key('G'):
		e.SetPosMaxY()
	case Key('D'):
		e.DeleteRow(e.CY())
	case Key('C'):
		e.SetRow(e.cy, "")
	case Key('w'):
		e.forwardWord()
	case Key('b'):
		e.backWord()
	}

	return nil
}

func (e *Editor) forwardWord() {
	i, ok := e.FindRight(func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if !ok {
		e.SetPosX(len(e.rows[e.cy].chars))
		return
	}

	e.SetPosX(i)

	i, _ = e.FindRight(func(r rune) bool {
		return r != ' ' && r != '\t'
	})

	e.SetPosX(i)
}

func (e *Editor) backWord() {
	i, ok := e.FindLeft(func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if !ok {
		e.SetPosX(0)
		return
	}

	// If the cursor is already at the beginning of the word, go to
	// the beginning of the next word
	if i == e.cx-1 {
		i, _ = e.FindLeft(func(r rune) bool {
			return r != ' ' && r != '\t'
		})
		e.SetPosX(i)

		i, _ = e.FindLeft(func(r rune) bool {
			return r == ' ' || r == '\t'
		})

		e.SetPosX(i)
	}

	e.SetPosX(i + 1)
}

func (e *Editor) drawStatusBar(b io.Writer) {
	setColor(b, InvertedColor)
	defer clearFormatting(b)

	filename := e.filename
	if len(filename) == 0 {
		filename = "[No Name]"
	}

	dirtyStatus := ""
	if e.modified {
		dirtyStatus = "(modified)"
	}

	mode := ""
	switch e.Mode {
	case InsertMode:
		mode = "-- INSERT MODE --"
	case CommandMode:
		mode = "-- COMMAND MODE --"
	}

	lmsg := fmt.Sprintf("%.20s - %d lines %s %s", filename, len(e.rows), dirtyStatus, mode)
	if runewidth.StringWidth(lmsg) > e.screenCols {
		lmsg = runewidth.Truncate(lmsg, e.screenCols, "...")
	}
	b.Write([]byte(lmsg))

	filetype := "no filetype"
	if e.syntax != nil {
		filetype = e.syntax.filetype
	}
	rmsg := fmt.Sprintf("%s | %d/%d", filetype, e.cy+1, len(e.rows))

	// Add padding between the left and right message
	l := runewidth.StringWidth(lmsg)
	r := runewidth.StringWidth(rmsg)
	for i := 0; i < e.screenCols-l-r; i++ {
		b.Write([]byte{' '})
	}

	b.Write([]byte(rmsg))
	b.Write([]byte("\r\n"))
}
