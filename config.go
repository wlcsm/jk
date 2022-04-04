package main

import (
	"fmt"
	"log"
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
		e.FindInteractive()
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

	e.InsertChars(e.CY(), e.CX(), rune(k))
	e.SetPosX(e.CX() + 1)

	return true, nil
}

var insertModeMapping = map[Key]func(e SDK) error{
	keyEnter: func(e SDK) error {
		row := e.Row(e.CY())
		row, row2 := row[:e.CX()], row[e.CX():]

		e.SetRow(e.CY(), row)
		e.InsertRow(e.CY()+1, row2)

		e.SetPosY(e.CY() + 1)
		e.SetPosX(0)

		return nil
	},
	keyCarriageReturn: func(e SDK) error {
		row := e.Row(e.CY())
		row, row2 := row[:e.CX()], row[e.CX():]

		e.SetRow(e.CY(), row)
		e.InsertRow(e.CY()+1, row2)

		e.SetPosY(e.CY() + 1)
		e.SetPosX(0)

		return nil
	},
	keyDelete: func(e SDK) error {
		x, y := e.CX(), e.CY()
		if x != 0 {
			e.Delete(y, x-1, x-1)
			e.SetPosX(x - 1)
		} else {
			e.SetPosY(y - 1)
			e.SetPosX(len(e.Row(y - 1)))

			e.SetRow(y-1, append(e.Row(y-1), e.Row(y)...))
			e.DeleteRow(y)
		}

		return nil
	},
	keyBackspace: func(e SDK) error {
		x, y := e.CX(), e.CY()
		if x != 0 {
			e.Delete(y, x-1, x-1)
			e.SetPosX(x - 1)
		} else {
			e.SetPosY(y - 1)
			e.SetPosX(len(e.Row(y - 1)))

			e.SetRow(y-1, append(e.Row(y-1), e.Row(y)...))
			e.DeleteRow(y)
		}

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
		e.InsertRow(e.CY()+1, []rune(""))
		e.SetPosY(e.CY() + 1)
		e.SetMode(InsertMode)
		return nil
	},
	Key('0'): func(e SDK) error {
		e.SetPosX(0)
		return nil
	},
	Key('$'): func(e SDK) error {
		e.SetPosX(len(e.Row(e.CY())))
		return nil
	},
	Key('G'): func(e SDK) error {
		e.SetPosY(e.NumRows())
		return nil
	},
	Key('D'): func(e SDK) error {
		e.DeleteRow(e.CY())
		return nil
	},
	Key('C'): func(e SDK) error {
		e.SetRow(e.CY(), []rune(""))
		return nil
	},
	Key('w'): func(e SDK) error {
		e.SetPosX(e.Word())
		return nil
	},
	Key('b'): func(e SDK) error {
		e.SetPosX(e.BackWord())
		return nil
	},
	Key('n'): func(e SDK) error {
		if len(e.LastSearch()) == 0 {
			e.SetMessage("There is no last search")
			return nil
		}

		// e.CX()+1 not e.CX() because we want to find the next match,
		// if we used e.CX() if the cursor was currently on a match it
		// would never move
		x, y := e.CX()+1, e.CY()
		if row := e.Row(y); x > len(row) {
			log.Printf("h x, y: %d, %d", x, y)
			if y == e.NumRows()-1 {
				return nil
			}

			x = 0
			y++
		}

		log.Printf("lastSearch: %s, x, y: %d, %d", string(e.LastSearch()), x, y)
		x, y = e.Find(x, y, e.LastSearch())
		log.Printf("x, y: %d, %d", x, y)
		if x != -1 {
			e.SetPosX(x)
			e.SetPosY(y)
		}

		return nil
	},
	Key('N'): func(e SDK) error {
		if len(e.LastSearch()) == 0 {
			e.SetMessage("There is no last search")
			return nil
		}

		// e.CX()-1 not e.CX() because we want to find the previous match,
		// if we used e.CX() if the cursor was currently on a match it
		// would never move
		x, y := e.CX()-1, e.CY()
		if x < 0 {
			if y == 0 {
				return nil
			}

			y--
			x = len(e.Row(y))
		}

		x, y = e.FindBack(x, y, e.LastSearch())
		log.Printf("x, y: %d, %d", x, y)
		if x != -1 {
			e.SetPosY(y)
			e.SetPosX(x)
		}

		return nil
	},
}
