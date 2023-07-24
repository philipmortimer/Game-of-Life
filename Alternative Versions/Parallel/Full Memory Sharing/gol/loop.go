package gol

import (
	"github.com/veandco/go-sdl2/sdl"
)

func SdlRun(p Params, events *EventHandler, keyPresses *KeyHandler) {
	w := NewWindow(int32(p.ImageWidth), int32(p.ImageHeight))
	go events.HandleEvents(w)

	for {
		event := w.PollEvent()
		if event != nil {
			switch e := event.(type) {
			case *sdl.KeyboardEvent:
				switch e.Keysym.Sym {
				case sdl.K_p:
					keyPresses.SendKey('p')
				case sdl.K_s:
					keyPresses.SendKey('s')
				case sdl.K_q:
					keyPresses.SendKey('q')
				case sdl.K_k:
					keyPresses.SendKey('k')
				}
			}
		}
	}
}
