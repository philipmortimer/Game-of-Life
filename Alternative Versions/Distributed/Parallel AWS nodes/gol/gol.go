package gol

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// Run starts the processing of Game of Life. It should initialise channels and goroutines.
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {
	// Channels required are created here
	ioCommand := make(chan ioCommand, 1)
	ioIdle := make(chan bool, 1)
	filename := make(chan string, 1)
	output := make(chan byte, p.ImageHeight*p.ImageWidth)
	input := make(chan byte, p.ImageHeight*p.ImageWidth)

	ioChannels := ioChannels{
		command:  ioCommand,
		idle:     ioIdle,
		filename: filename,
		output:   output,
		input:    input,
	}
	go startIo(p, ioChannels)

	distributorChannels := distributorChannels{
		events:     events,
		ioCommand:  ioCommand,
		ioIdle:     ioIdle,
		ioFilename: filename,
		ioOutput:   output,
		ioInput:    input,
	}
	distributor(p, distributorChannels, keyPresses)
}
