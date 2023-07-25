package gol

import (
	"github.com/ChrisGora/semaphore"
	"sync"
)

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// IoGrid used to send a grid to be written to file system or reads a grid in from file
type IoGrid struct {
	Grid          [][]byte
	FileName      string
	CommandSend   semaphore.Semaphore
	CommandReturn semaphore.Semaphore
}

// initIoOut initialises IoGrid structure with correct values
func initIoGrid(p Params) *IoGrid {
	return &IoGrid{
		Grid:          makeTwoDMatrix(p.ImageHeight, p.ImageWidth),
		FileName:      "",
		CommandSend:   semaphore.Init(1, 0),
		CommandReturn: semaphore.Init(1, 0),
	}
}

// Run starts the processing of Game of Life. It should initialise channels and goroutines.
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {
	ioChannels := ioChannels{
		CommandBeingExecuted: &sync.Mutex{},
		IoInput:              initIoGrid(p),
		IoOutput:             initIoGrid(p),
	}
	go startIo(p, ioChannels)

	distributorChannels := distributorChannels{
		events:     events,
		IoChannels: ioChannels,
	}
	distributor(p, distributorChannels, keyPresses)
}
