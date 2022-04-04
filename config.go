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
	switch k {
	case keyPageUp:
		e.SetY(e.ScreenTop())
	case keyPageDown:
		e.SetY(e.ScreenBottom())
	case keyArrowUp:
		e.SetY(e.Y() - 1)
	case keyArrowDown:
		e.SetY(e.Y() + 1)
	case keyArrowLeft:
		e.SetX(e.X() - 1)
	case keyArrowRight:
		e.SetX(e.X() + 1)
	case Key(ctrl('q')):
		if e.IsModified() {
			e.Prompt("WARNING!!! File has unsaved changes. Press Ctrl-Q again to quit.",
				func(k Key) (string, bool) {
					log.Printf("im here now")
					if k == Key(ctrl('q')) {
						e.ErrChan() <- ErrQuitEditor
					}

					return "", true
				})
		} else {
			ClearScreen()
			RepositionCursor()

			return true, ErrQuitEditor
		}
	case Key(ctrl('s')):
		log.Printf("attempting to save: %s\n", e.Filename())
		if err := e.Save(); err != nil {
			return true, err
		}

		log.Println("should have saved")
		e.SetMessage("saved file: %s", e.Filename())

	case Key(ctrl('e')):
		e.StaticPrompt("File name: ", func(res string) error {
			if len(res) == 0 {
				return fmt.Errorf("No file name")
			}

			return e.OpenFile(res)
		}, FileCompletion)

	case Key(ctrl('f')):
		e.FindInteractive()
	case Key(ctrl('w')):
		e.Delete(e.Y(), e.BackWord(), e.X()-1)
	case Key(ctrl('r')):
		return true, RestartEditor
	case Key(ctrl('u')):
		e.SetY(e.Y() - (e.Rows() / 2))
		e.CenterCursor()
	case Key(ctrl('d')):
		e.SetY(e.Y() + (e.Rows() / 2))
		e.CenterCursor()
	default:
		return false, nil
	}

	return true, nil
}

var InsertModeMap = KeyMap{
	Name:    InsertModeName,
	Handler: insertModeHandler,
}

func insertModeHandler(e SDK, k Key) (bool, error) {
	switch k {
	case keyEnter:
		row := e.Row(e.Y())
		row, row2 := row[:e.X()], row[e.X():]

		e.SetRow(e.Y(), row)
		e.InsertRow(e.Y()+1, row2)

		e.SetY(e.Y() + 1)
		e.SetX(0)

	case keyCarriageReturn:
		row := e.Row(e.Y())
		row, row2 := row[:e.X()], row[e.X():]

		e.SetRow(e.Y(), row)
		e.InsertRow(e.Y()+1, row2)

		e.SetY(e.Y() + 1)
		e.SetX(0)

	case keyDelete:
		x, y := e.X(), e.Y()
		if x != 0 {
			e.Delete(y, x-1, x-1)
			e.SetX(x - 1)
		} else {
			e.SetY(y - 1)
			e.SetX(len(e.Row(y - 1)))

			e.SetRow(y-1, append(e.Row(y-1), e.Row(y)...))
			e.DeleteRow(y)
		}

	case keyBackspace:
		x, y := e.X(), e.Y()
		if x != 0 {
			e.Delete(y, x-1, x-1)
			e.SetX(x - 1)
		} else {
			e.SetY(y - 1)
			e.SetX(len(e.Row(y - 1)))

			e.SetRow(y-1, append(e.Row(y-1), e.Row(y)...))
			e.DeleteRow(y)
		}

	case Key(ctrl('c')):
		e.SetMode(CommandMode)
	default:
		if isPrintable(k) {
			e.InsertChars(e.Y(), e.X(), rune(k))
			e.SetX(e.X() + 1)
		}
	}

	return true, nil
}

var CommandModeMap = KeyMap{
	Name:    CommandModeName,
	Handler: commandModeHandler,
}

func commandModeHandler(e SDK, k Key) (bool, error) {
	switch k {
	case Key('j'):
		e.SetY(e.Y() + 1)
	case Key('k'):
		e.SetY(e.Y() - 1)
	case Key('h'):
		e.SetX(e.X() - 1)
	case Key('l'):
		e.SetX(e.X() + 1)
	case Key('i'):
		e.SetMode(InsertMode)
	case Key('o'):
		e.InsertRow(e.Y()+1, []rune(""))
		e.SetY(e.Y() + 1)
		e.SetMode(InsertMode)
	case Key('0'):
		e.SetX(0)
	case Key('$'):
		e.SetX(len(e.Row(e.Y())))
	case Key('G'):
		e.SetY(e.NumRows())
	case Key('D'):
		e.DeleteRow(e.Y())
	case Key('C'):
		e.SetRow(e.Y(), []rune(""))
	case Key('w'):
		e.SetX(e.Word())
	case Key('b'):
		e.SetX(e.BackWord())
	case Key('n'):
		if len(e.LastSearch()) == 0 {
			e.SetMessage("There is no last search")
			break
		}

		// e.X()+1 not e.X() because we want to find the next match,
		// if we used e.X() if the cursor was currently on a match it
		// would never move
		x, y := e.X()+1, e.Y()
		if row := e.Row(y); x > len(row) {
			log.Printf("h x, y: %d, %d", x, y)
			if y == e.NumRows()-1 {
				break
			}

			x = 0
			y++
		}

		log.Printf("lastSearch: %s, x, y: %d, %d", string(e.LastSearch()), x, y)
		x, y = e.Find(x, y, e.LastSearch())
		log.Printf("x, y: %d, %d", x, y)
		if x != -1 {
			e.SetX(x)
			e.SetY(y)
		}
	case Key('N'):
		if len(e.LastSearch()) == 0 {
			e.SetMessage("There is no last search")
			break
		}

		// e.X()-1 not e.X() because we want to find the previous match,
		// if we used e.X() if the cursor was currently on a match it
		// would never move
		x, y := e.X()-1, e.Y()
		if x < 0 {
			if y == 0 {
				break
			}

			y--
			x = len(e.Row(y))
		}

		x, y = e.FindBack(x, y, e.LastSearch())
		log.Printf("x, y: %d, %d", x, y)
		if x != -1 {
			e.SetY(y)
			e.SetX(x)
		}
	default:
		return false, nil
	}

	return true, nil
}
