package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mattn/go-runewidth"
	"github.com/pkg/errors"
)

const Version = "dev"

type KeyMap struct {
	Name    KeyMapName
	Handler func(e SDK, k Key) (bool, error)
}

// Mappings at the beginning have higher priority
var Keymapping = []KeyMap{
	BasicMap,
	CommandModeMap,
}

var KeyModes = map[KeyMapName]KeyMap{
	BasicMapName:    BasicMap,
	InsertModeName:  InsertModeMap,
	CommandModeName: CommandModeMap,
}

type KeyMapName int

const (
	BasicMapName KeyMapName = iota + 1
	InsertModeName
	CommandModeName
)

var BasicMap = KeyMap{
	Name:    BasicMapName,
	Handler: basicHandler,
}

func basicHandler(e SDK, k Key) (bool, error) {
	if f, ok := basicMapping[k]; ok {
		return true, f(e)
	}

	return false, nil
}

var basicMapping = map[Key]func(e SDK) error{
	keyBackspace: func(e SDK) error {
		e.DeleteChar()
		return nil
	},
	keyPageUp: func(e SDK) error {
		e.SetPosY(e.ScreenTop())
		return nil
	},
	keyPageDown: func(e SDK) error {
		e.SetPosY(e.ScreenBottom())
		return nil
	},
	keyArrowUp: func(e SDK) error {
		e.SetPosY(e.CY() - 1)
		return nil
	},
	keyArrowDown: func(e SDK) error {
		e.SetPosY(e.CY() + 1)
		return nil
	},
	keyArrowLeft: func(e SDK) error {
		e.SetPosX(e.CX() - 1)
		return nil
	},
	keyArrowRight: func(e SDK) error {
		e.SetPosX(e.CX() + 1)
		return nil
	},
	Key(ctrl('q')): func(e SDK) error {
		if e.IsModified() {
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
	},
	Key(ctrl('s')): func(e SDK) error {
		log.Printf("attempting to save: %s\n", e.Filename())
		if err := e.Save(); err != nil {
			return err
		}

		log.Println("should have saved")
		e.SetMessage("saved file: %s", e.Filename())
		return nil
	},
	// Open a new file
	Key(ctrl('e')): func(e SDK) error {
		filename, err := e.StaticPrompt("File name: ", FileCompletion)
		if errors.Is(err, ErrPromptCanceled) {
			return nil
		}

		if err != nil {
			return err
		}

		if len(filename) == 0 {
			return fmt.Errorf("No file name")
		}

		if err = e.OpenFile(filename); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("File doesn't exist")
		}

		return err
	},
	Key(ctrl('f')): func(e SDK) error {
		err := e.Find()
		if err == ErrPromptCanceled {
			e.SetMessage("")
		}

		if err != nil {
			return err
		}
		return nil
	},
	Key(ctrl('w')): func(e SDK) error {
		e.DeleteUntil(e.BackWord())
		return nil
	},
	Key(ctrl('w')): func(e SDK) error {
		e.DeleteUntil(e.BackWord())
		return nil
	},
	Key(ctrl('r')): func(e SDK) error {
		return RestartEditor
	},
	Key(ctrl('u')): func(e SDK) error {
		e.SetPosY(e.CY() - (e.Rows() / 2))
		e.CenterCursor()
		return nil
	},
	Key(ctrl('d')): func(e SDK) error {
		e.SetPosY(e.CY() + (e.Rows() / 2))
		e.CenterCursor()
		return nil
	},
}

var InsertModeMap = KeyMap{
	Name:    InsertModeName,
	Handler: insertModeHandler,
}

func insertModeHandler(e SDK, k Key) (bool, error) {
	if f, ok := insertModeMapping[k]; ok {
		err := f(e)
		return true, err
	}

	e.InsertChar(rune(k))

	return true, nil
}

var insertModeMapping = map[Key]func(e SDK) error{
	keyEnter: func(e SDK) error {
		e.InsertNewline()
		return nil
	},
	keyCarriageReturn: func(e SDK) error {
		e.InsertNewline()
		return nil
	},
	Key(ctrl('c')): func(e SDK) error {
		e.SetMode(CommandMode)
		return nil
	},
}

var CommandModeMap = KeyMap{
	Name:    CommandModeName,
	Handler: commandModeHandler,
}

func commandModeHandler(e SDK, k Key) (bool, error) {
	if f, ok := commandModeMapping[k]; ok {
		err := f(e)
		return true, err
	}

	return false, nil
}

var commandModeMapping = map[Key]func(e SDK) error{
	Key('j'): func(e SDK) error {
		e.SetPosY(e.CY() + 1)
		return nil
	},
	Key('k'): func(e SDK) error {
		e.SetPosY(e.CY() - 1)
		return nil
	},
	Key('h'): func(e SDK) error {
		e.SetPosX(e.CX() - 1)
		return nil
	},
	Key('l'): func(e SDK) error {
		e.SetPosX(e.CX() + 1)
		return nil
	},
	Key('i'): func(e SDK) error {
		e.SetMode(InsertMode)
		return nil
	},
	Key('o'): func(e SDK) error {
		e.InsertRow(e.CY()+1, "")
		e.SetPosY(e.CY() + 1)
		e.SetMode(InsertMode)
		return nil
	},
	Key('0'): func(e SDK) error {
		e.SetPosX(0)
		return nil
	},
	Key('$'): func(e SDK) error {
		e.SetPosMaxX()
		return nil
	},
	Key('G'): func(e SDK) error {
		e.SetPosMaxY()
		return nil
	},
	Key('D'): func(e SDK) error {
		e.DeleteRow(e.CY())
		return nil
	},
	Key('C'): func(e SDK) error {
		e.SetRow(e.CY(), "")
		return nil
	},
	Key('w'): func(e SDK) error {
		e.ForwardWord()
		return nil
	},
	Key('b'): func(e SDK) error {
		e.SetPosX(e.BackWord())
		return nil
	},
}

func (e *Editor) ForwardWord() {
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

func (e *Editor) BackWord() int {
	x, y := e.CX(), e.CY()
	i, _ := e.FindLeft(x, y, func(r rune) bool {
		return r == ' ' || r == '\t'
	})

	// If the cursor is already at the beginning of the word, go to
	// the beginning of the next word
	if i == x-1 {
		i, _ = e.FindLeft(i, y, func(r rune) bool {
			return r != ' ' && r != '\t'
		})

		i, _ = e.FindLeft(i, y, func(r rune) bool {
			return r == ' ' || r == '\t'
		})
	}

	return i
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
