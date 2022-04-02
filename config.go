package main

import (
	"fmt"
	"io"
	"log"
	"unicode"

	"github.com/mattn/go-runewidth"
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

func SetKeymapping(k []KeyMap) {
	Keymapping = k
}

var KeyModes = map[KeyMapName]KeyMap{
	BasicMapName:    BasicMap,
	InsertModeName:  InsertModeMap,
	CommandModeName: CommandModeMap,
}

type KeyMapName string

const (
	BasicMapName    KeyMapName = "Basic"
	InsertModeName  KeyMapName = "Insert"
	CommandModeName KeyMapName = "Command"
	PromptModeName  KeyMapName = "Prompt"
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
		x := e.CX()
		if x != 0 {
			e.Delete(e.CY(), x-1, x-1)
		}

		e.SetPosX(x - 1)

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
			e.Prompt("WARNING!!! File has unsaved changes. Press Ctrl-Q again to quit.",
				func(k Key) (string, bool) {
					log.Printf("im here now")
					if k == Key(ctrl('q')) {
						e.ErrChan() <- ErrQuitEditor
					}

					return "", true
				})

			return nil
		} else {
			ClearScreen()
			RepositionCursor()

			return ErrQuitEditor
		}
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
		e.StaticPrompt("File name: ", func(res string) error {
			if len(res) == 0 {
				return fmt.Errorf("No file name")
			}

			return e.OpenFile(res)
		}, FileCompletion)

		return nil
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
		e.Delete(e.CY(), e.BackWord(), e.CX()-1)
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

func (e *Editor) ForwardWord() int {
	x, y := e.CX(), e.CY()
	row := e.rows[y].chars

	i := Find(row[x:], unicode.IsSpace)
	if i == -1 {
		return len(row)
	}

	i = Find(row[i:], func(r rune) bool { return !unicode.IsSpace(r) })
	if i == -1 {
		return len(row)
	}

	return i
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
	for i := len(s) - 1; i > 0; i++ {
		if f(s[i]) {
			return i
		}
	}

	return -1
}

func (e *Editor) BackWord() int {
	x, y := e.CX(), e.CY()
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
