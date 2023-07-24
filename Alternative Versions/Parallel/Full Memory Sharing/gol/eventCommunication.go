package gol

import (
	"fmt"
	"github.com/ChrisGora/semaphore"
	"uk.ac.bris.cs/gameoflife/util"
)

/* Handles communication between sdl and distributor without using channels */

// EventHandler handles the sending and receipt of events
type EventHandler struct {
	EventDetails          Event
	EventHasBeenProcessed semaphore.Semaphore
	EventToProcess        semaphore.Semaphore
	LastTurnReceived      semaphore.Semaphore // Used to indicate that final turn complete event has been received
	StartedProcessing     semaphore.Semaphore
	Wind                  *Window
	FinalAliveCells       []util.Cell
}

// InitEventHandler initialises an event handling struct
func InitEventHandler() *EventHandler {
	return &EventHandler{
		EventDetails:          nil,
		EventHasBeenProcessed: semaphore.Init(1, 1),
		EventToProcess:        semaphore.Init(1, 0),
		LastTurnReceived:      semaphore.Init(1, 0),
		StartedProcessing:     semaphore.Init(1, 0),
		Wind:                  nil,
	}
}

func (ev *EventHandler) SendEvent(event Event) {
	ev.EventHasBeenProcessed.Wait()
	ev.EventDetails = event
	ev.EventToProcess.Post()
}

// WaitForHandlerToStart waits for handling of events to start
func (ev *EventHandler) WaitForHandlerToStart() {
	ev.StartedProcessing.Wait()
}

func (ev *EventHandler) FlipCell(cell util.Cell) {
	if ev.Wind != nil {
		ev.Wind.FlipPixel(cell.X, cell.Y)
	}
}

// HandleEvents listens for events in a loop and handles them as required
func (ev *EventHandler) HandleEvents(w *Window) {
	ev.Wind = w
	ev.StartedProcessing.Post()
	quit := false
	for !quit {
		ev.EventToProcess.Wait()
		event := ev.EventDetails
		ev.EventHasBeenProcessed.Post()
		switch e := event.(type) {
		case CellFlipped:
			//w.FlipPixel(e.Cell.X, e.Cell.Y)
		case TurnComplete:
			if w != nil {
				w.RenderFrame()
			}
		case FinalTurnComplete:
			ev.FinalAliveCells = e.Alive
			ev.LastTurnReceived.Post()
		default:
			if w != nil && len(event.String()) > 0 {
				fmt.Printf("Completed Turns %-8v%v\n", event.GetCompletedTurns(), event)
			}
		}
	}
}
