package gol

import (
	"fmt"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// WorldChunk Stores a chunk of GoL board that can be used by worker to calculate the next state
type WorldChunk struct {
	Chunk    [][]byte
	AboveRow []byte
	BelowRow []byte
	StartY   int
}

const Alive byte = 255
const Dead byte = 0
const AboveRowIndex = -1 // Used to indicate that cell needs to be read from a ghost row
const BelowRowIndex = -2

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	ghostRowUpdates := make(chan bool, p.Threads) // Channel used to concurrently process ghost row updates
	// Two world structures are used to store the current and next boards
	world, ghostRowUpdates := getBoard(p, c)  // Reads in GoL life grid
	nextWorld := dimensionalCopyWorld(world)  // Dimensional copy of world
	ticker := time.NewTicker(2 * time.Second) // Ticker used to send list of alive cells every 2 seconds
	// Executes all turns of the Game of Life. Also handles various other events
	turn := 0
	paused := false
	quit := false
	for turn < p.Turns && !quit {
		select {
		case charPressed := <-keyPresses: // Handles key presses
			handleKeyPress(charPressed, world, p, c, &paused, &quit, turn)
		case <-ticker.C:
			if !paused { // Sends number of alive cells
				c.events <- AliveCellsCount{CellsCount: noAliveCells(world), CompletedTurns: turn}
			}
		default:
			if !paused { // By default, will evolve board state by one turn
				world, nextWorld = evolveGrid(world, nextWorld, &turn, c.events, ghostRowUpdates, c)
			}
		}
	}
	writeImage(world, c, p, turn) // Outputs pgm image after all turns completed
	ticker.Stop()                 // Stops ticker
	gameFinished(c, turn, world)  // Ensures GoL finish occurs correctly
}

// handleKeyPress handles key presses made by user in SDL window. If 's' is pressed, game state is saved.
// 'q' saves an image and quits the main game loop. 'p' pauses / unpauses the game. The quit and paused
// boolean variables are updated accordingly.
func handleKeyPress(charPressed rune,
	world []WorldChunk, p Params, c distributorChannels, paused, quit *bool, turn int) {
	if charPressed == 's' {
		writeImage(world, c, p, turn) // Saves image
	} else if charPressed == 'p' {
		// Pauses or unpauses game
		*paused = !*paused
		newState := Paused
		if !*paused {
			newState = Executing
			fmt.Println("Continuing") // Print statements look ugly but seem to be required by CW spec
		}
		c.events <- StateChange{CompletedTurns: turn, NewState: newState}
	} else if charPressed == 'q' {
		*quit = true
	}
}

// gameFinished gracefully exits game by ensuring all IO process are done and that
// final turn being completed is reported.
func gameFinished(c distributorChannels, turn int, world []WorldChunk) {
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	// Reports the final state and quits
	c.events <- FinalTurnComplete{turn, aliveCells(world)}
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// updateGhostRows updates the above and below ghost rows of the previous and next chunk with its row data.
// Writes true to channel once update is complete
func updateGhostRows(chunkIndex int, newWorld []WorldChunk, rowUpdated chan<- bool) {
	// Updates below row of previous chunk
	prevChunkIndex := chunkIndex - 1
	if prevChunkIndex < 0 {
		prevChunkIndex = len(newWorld) - 1
	}
	newWorld[prevChunkIndex].BelowRow = newWorld[chunkIndex].Chunk[0]

	// Updates above row of next chunk
	nextChunkIndex := (chunkIndex + 1) % len(newWorld)
	newWorld[nextChunkIndex].AboveRow = newWorld[chunkIndex].Chunk[len(newWorld[chunkIndex].Chunk)-1]
	rowUpdated <- true // Indicates that row has been updated
}

// evolveChunk evolves the provided GoL chunk
func evolveChunk(newWorld []WorldChunk, currentChunk WorldChunk, chunkIndex, turn int, events chan<- Event,
	chunkUpdated chan<- bool) {
	newWorld[chunkIndex].StartY = currentChunk.StartY
	// Updates grid of chunk
	nextState(newWorld[chunkIndex], currentChunk, turn, events)
	// Updates above and below rows of chunk
	updateGhostRows(chunkIndex, newWorld, chunkUpdated)
}

// evolveGrid evolves GoL grid by 1 turn and returns new grid
func evolveGrid(world []WorldChunk, newWorld []WorldChunk, turn *int, events chan<- Event, chunkUpdated chan bool,
	c distributorChannels) ([]WorldChunk, []WorldChunk) {
	// Starts each of worker threads
	for i := range world {
		go evolveChunk(newWorld, world[i], i, *turn, events, chunkUpdated)
	}
	// Ensures that all chunks board data has been updated
	for range world {
		<-chunkUpdated
	}
	// Updates variables used in main GoL loop
	*turn++
	c.events <- TurnComplete{*turn}
	// Updates which structure stores current world and which one will store next world.
	return newWorld, world
}

// writeImage writes the provided GoL board as PGM file
func writeImage(world []WorldChunk, c distributorChannels, p Params, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	// Writes grid to channel
	for chunkIndex := range world {
		for _, row := range world[chunkIndex].Chunk {
			for _, cont := range row {
				c.ioOutput <- cont
			}
		}
	}
}

// dimensionalCopyWorld returns a copy of the provided array with same dimensions.
// Used to initialise an empty world structure.
func dimensionalCopyWorld(world []WorldChunk) []WorldChunk {
	newWorld := make([]WorldChunk, len(world))
	for i := range world {
		newWorld[i] = WorldChunk{Chunk: makeTwoDMatrix(len(world[i].Chunk), len(world[i].Chunk[0])),
			AboveRow: nil, BelowRow: nil, StartY: -1}
	}
	return newWorld
}

// initEmptyChunkStruct initialises the array of chunks used to process GoL.
func initEmptyChunkStruct(p Params) []WorldChunk {
	// Divides board evenly amongst thread by giving each thread roughly the same number of rows to process
	// If there are more threads than rows, some threads don't do any processing.
	rowsPerThread := p.ImageHeight / p.Threads
	if rowsPerThread < 1 {
		rowsPerThread = 1
	}
	noProcessesNeeded := p.ImageHeight / rowsPerThread // Works out number of threads actually used
	// Adds extra process for case when rows can't be evenly shared by threads
	if p.ImageHeight%rowsPerThread != 0 {
		noProcessesNeeded++
	}

	// Initialises WorldChunk data structures
	chunks := make([]WorldChunk, noProcessesNeeded)
	totalRowsAssigned := 0 // Tracks number of rows actually assigned already for processing
	for chunkIndex := 0; chunkIndex < len(chunks); chunkIndex++ {
		// Calculates number of rows for process to deal with
		height := rowsPerThread
		if height+totalRowsAssigned > p.ImageHeight {
			height = p.ImageHeight - totalRowsAssigned
		}
		chunks[chunkIndex] = WorldChunk{makeTwoDMatrix(height, p.ImageWidth),
			nil, nil, totalRowsAssigned}
		totalRowsAssigned += height
	}
	return chunks
}

// getBoard gets the GoL board data and splits into chunks for threads to process.
// Also returns a boolean channel with buffer size equal to number of chunks
func getBoard(p Params, c distributorChannels) ([]WorldChunk, chan bool) {
	// Tells IO to read appropriate image in
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)

	chunks := initEmptyChunkStruct(p)
	ghostRowUpdates := make(chan bool, len(chunks))
	// Reads in GoL board data from IO into chunk matrix
	for chunkIndex := 0; chunkIndex < len(chunks); chunkIndex++ {
		for y := 0; y < len(chunks[chunkIndex].Chunk); y++ {
			for x := 0; x < p.ImageWidth; x++ {
				chunks[chunkIndex].Chunk[y][x] = <-c.ioInput
				// Sends cell flipped event for all cells that start off as alive
				if chunks[chunkIndex].Chunk[y][x] == Alive {
					c.events <- CellFlipped{Cell: util.Cell{X: x, Y: y + chunks[chunkIndex].StartY}, CompletedTurns: 0}
				}
			}
		}
		go updateGhostRows(chunkIndex, chunks, ghostRowUpdates)
	}
	// Ensures that all ghost rows are correctly updated
	for range chunks {
		<-ghostRowUpdates
	}
	return chunks, ghostRowUpdates
}

// makeTwoDMatrix makes a two d slice of given height and width
func makeTwoDMatrix(height, width int) [][]byte {
	x := make([][]byte, height)
	for i := range x {
		x[i] = make([]byte, width)
	}
	return x
}

// cellAlive checks to see if cell specified is alive.
// 1 is returned if cell is alive, 0 otherwise
func cellAlive(y, x int, world WorldChunk) int {
	var cell byte
	if y == AboveRowIndex {
		cell = world.AboveRow[x]
	} else if y == BelowRowIndex {
		cell = world.BelowRow[x]
	} else {
		cell = world.Chunk[y][x]
	}
	if cell == Alive {
		return 1
	} else {
		return 0
	}
}

// aliveNeighbours calculates the number of alive neighbours for a given cell
func aliveNeighbours(y, x int, world WorldChunk) int {
	height := len(world.Chunk)
	width := len(world.Chunk[0])
	// Calculates indices of neighbours (wrapping corner if needed)
	below := y + 1
	if below >= height {
		below = BelowRowIndex
	}
	above := y - 1
	if above < 0 {
		above = AboveRowIndex
	}
	right := (x + 1) % width
	left := x - 1
	if left < 0 {
		left = width - 1
	}
	// Calculates alive neighbors
	alive := cellAlive(y, left, world) + cellAlive(above, left, world) + cellAlive(below, left, world) +
		cellAlive(y, right, world) + cellAlive(above, right, world) + cellAlive(below, right, world) +
		cellAlive(above, x, world) + cellAlive(below, x, world)
	return alive
}

// nextState Runs a single iteration of GoL algorithm on a chunk of grid
func nextState(newChunk WorldChunk, currentChunk WorldChunk, turn int, events chan<- Event) {
	// Loops through cell's and applies GoL rules to calculate new state
	for y, row := range currentChunk.Chunk {
		for x, cellCont := range row {
			neighboursAlive := aliveNeighbours(y, x, currentChunk)
			// Updates new world cell in chunk based on calculation
			if cellCont == Alive && neighboursAlive < 2 {
				newChunk.Chunk[y][x] = Dead
			} else if cellCont == Alive && neighboursAlive > 3 {
				newChunk.Chunk[y][x] = Dead
			} else if cellCont == Dead && neighboursAlive == 3 {
				newChunk.Chunk[y][x] = Alive
			} else {
				newChunk.Chunk[y][x] = cellCont
			}
			// Sends cell flipped notification
			if newChunk.Chunk[y][x] != cellCont {
				events <- CellFlipped{Cell: util.Cell{X: x, Y: y + currentChunk.StartY}, CompletedTurns: turn}
			}
		}
	}
}

// noAliveCells calculates number of alive cells
func noAliveCells(world []WorldChunk) int {
	alive := 0
	// Loops through all cells and calculates number of alive cells
	for chunkIndex := range world {
		for _, row := range world[chunkIndex].Chunk {
			for _, cell := range row {
				if cell == Alive {
					alive++
				}
			}
		}
	}
	return alive
}

// Calculates all alive cells in GoL world and returns a list of all alive cells.
func aliveCells(world []WorldChunk) []util.Cell {
	i := 0
	alive := make([]util.Cell, noAliveCells(world))
	// Loops through all cells and collates list of alive cells
	for chunkIndex := range world {
		for y, row := range world[chunkIndex].Chunk {
			for x, cell := range row {
				if cell == Alive {
					alive[i] = util.Cell{X: x, Y: y + world[chunkIndex].StartY}
					i++
				}
			}
		}
	}
	return alive
}
