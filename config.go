package main

const Version = "dev"

const Tabstop = 8

// The number of times the user needs to press Ctrl-Q to quit
// the editor with unsaved changes.
const QuitTimes = 0

func (e *Editor) Keymapping(k Key) error {
	switch k {
	case keyEnter:
		e.InsertNewline()

	case Key(ctrl('q')):
		// warn the user about unsaved changes.
		if e.dirty > 0 && e.quitCounter < QuitTimes {
			e.SetStatusMessage(
				"WARNING!!! File has unsaved changes. Press Ctrl-Q %d more times to quit.",
				QuitTimes-e.quitCounter,
			)
			e.quitCounter++
			return nil
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
		// position cursor at the top first.
		e.cy = e.rowOffset
		// then scroll up an entire screen worth.
		for i := 0; i < e.screenRows; i++ {
			e.SetRelativePosY(-1)
		}
	case keyPageDown:
		// position cursor at the bottom first.
		e.cy = e.rowOffset + e.screenRows - 1
		if e.cy > len(e.rows) {
			e.cy = len(e.rows)
		}
		// then scroll down an entire screen worth.
		for i := 0; i < e.screenRows; i++ {
			e.SetRelativePosY(1)
		}
	case keyArrowUp:
		e.SetRelativePosY(-1)
	case keyArrowDown:
		e.SetRelativePosY(1)
	case keyArrowLeft:
		e.SetRelativePosX(-1)
	case keyArrowRight:
		e.SetRelativePosX(1)

	case Key(ctrl('l')), Key('\x1b'):
		break // no op

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

	// Reset quitCounter to zero if user pressed any key other than Ctrl-Q.
	e.quitCounter = 0

	return nil
}

func (e *Editor) CX() int {
	return e.cx
}

func (e *Editor) CY() int {
	return e.cy
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
		e.SetRelativePosX(1)
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
		i, ok := e.FindRight(func(r rune) bool {
			return r == ' ' || r == '\t'
		})
		if !ok {
			e.SetPosX(len(e.rows[e.cy].chars))
			break
		}

		e.SetPosX(i)

		i, _ = e.FindRight(func(r rune) bool {
			return r != ' ' && r != '\t'
		})

		e.SetPosX(i)
	case Key('b'):
		i, ok := e.FindLeft(func(r rune) bool {
			return r == ' ' || r == '\t'
		})
		if !ok {
			e.SetPosX(0)
			break
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

	return nil
}
