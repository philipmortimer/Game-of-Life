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

// WorldChunk references a subset of a world. Used to divide number of rows between processes for GoL algo
type WorldChunk struct {
	StartY        int
	RowsProcessed int
}

// GameOfLifeData structure stores the structures needed to process GoL in parallel.
type GameOfLifeData struct {
	CurrentGrid  [][]byte
	NewGrid      [][]byte
	ThreadChunks []WorldChunk
	GridHeight   int
	GridWidth    int
}

const Alive byte = 255
const Dead byte = 0

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	// Two world structures are used to store the current and next boards
	algoData := getGolData(p, c)
	gridEvolved := make(chan bool, len(algoData.CurrentGrid))
	ticker := time.NewTicker(2 * time.Second) // Ticker used to send list of alive cells every 2 seconds*/
	// Executes all turns of the Game of Life. Also handles various other events
	turn := 0
	paused := false
	quit := false
	for turn < p.Turns && !quit {
		select {
		case charPressed := <-keyPresses: // Handles key presses
			handleKeyPress(charPressed, algoData.CurrentGrid, p, c, &paused, &quit, turn)
		case <-ticker.C:
			if !paused { // Sends number of alive cells
				c.events <- AliveCellsCount{CellsCount: noAliveCells(algoData.CurrentGrid), CompletedTurns: turn}
			}
		default:
			if !paused { // By default, will evolve board state by one turn and update grids
				evolveGrid(&algoData, &turn, c.events, gridEvolved)
			}
		}
	}
	writeImage(algoData.CurrentGrid, c, p, turn) // Outputs pgm image after all turns completed
	ticker.Stop()                                // Stops ticker
	gameFinished(c, turn, algoData.CurrentGrid)  // Ensures GoL finish occurs correctly
}

// getGolData gets all data needed to process game of life efficiently
func getGolData(p Params, c distributorChannels) GameOfLifeData {
	// Gets grid and dimensional copy of grid
	currWorld := getWorldGrid(p, c)
	newWorld := makeTwoDMatrix(p.ImageHeight, p.ImageWidth)

	// Divides grid by its rows into chunks to be processed by each thread
	chunks := divideGridForProcessing(p)

	return GameOfLifeData{CurrentGrid: currWorld, NewGrid: newWorld, ThreadChunks: chunks,
		GridHeight: p.ImageHeight, GridWidth: p.ImageWidth}
}

// divideGridForProcessing divides grid into roughly even chunks that can be processed by each thread.
func divideGridForProcessing(p Params) []WorldChunk {
	// Divides board evenly amongst thread by giving each thread roughly the same number of rows to process
	rowsPerThread := p.ImageHeight / p.Threads

	// Accounts for when number of rows don't nicely divide between number of threads
	if rowsPerThread == 0 { // If there are more threads than rows, some threads don't do any processing.
		rowsPerThread = 1
	}
	noProcessesNeeded := p.ImageHeight / rowsPerThread // Works out number of threads actually used
	extraRows := p.ImageHeight % rowsPerThread

	// Initialises WorldChunk data structures
	chunks := make([]WorldChunk, noProcessesNeeded)
	totalRowsAssigned := 0 // Tracks number of rows actually assigned already for processing
	for chunkIndex := 0; chunkIndex < noProcessesNeeded; chunkIndex++ {
		rowsToProcess := rowsPerThread
		if chunkIndex < extraRows {
			rowsToProcess++
		}
		chunks[chunkIndex] = WorldChunk{totalRowsAssigned, rowsToProcess}
		totalRowsAssigned += rowsToProcess
	}
	return chunks
}

// getWorldGrid gets the inputted grid from io
func getWorldGrid(p Params, c distributorChannels) [][]byte {
	// Tells IO to read appropriate image in
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)

	world := makeTwoDMatrix(p.ImageHeight, p.ImageWidth)
	// Reads image into slice
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
			if world[y][x] == Alive {
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
			}
		}
	}
	return world
}

// handleKeyPress handles key presses made by user in SDL window. If 's' is pressed, game state is saved.
// 'q' saves an image and quits the main game loop. 'p' pauses / unpauses the game. The quit and paused
// boolean variables are updated accordingly.
func handleKeyPress(charPressed rune,
	world [][]byte, p Params, c distributorChannels, paused, quit *bool, turn int) {
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
func gameFinished(c distributorChannels, turn int, world [][]byte) {
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	// Reports the final state and quits
	c.events <- FinalTurnComplete{turn, aliveCells(world)}
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// evolveGrid evolves GoL grid by 1 turn
func evolveGrid(grids *GameOfLifeData, turn *int, events chan<- Event, chunkUpdated chan bool) {
	// Starts each of worker threads
	for i := range grids.ThreadChunks {
		go nextState(grids.CurrentGrid, grids.NewGrid, grids.ThreadChunks[i], *turn, events, chunkUpdated,
			grids.GridHeight, grids.GridWidth)
	}
	// Ensures that all chunks board data has been updated
	for range grids.ThreadChunks {
		<-chunkUpdated
	}
	// Updates variables used in main GoL loop
	*turn++
	events <- TurnComplete{*turn}
	// Updates grids
	oldCurr := (*grids).CurrentGrid
	(*grids).CurrentGrid = (grids).NewGrid
	(*grids).NewGrid = oldCurr
}

// writeImage writes the provided GoL board as PGM file
func writeImage(world [][]byte, c distributorChannels, p Params, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	// Writes grid to channel
	for _, row := range world {
		for _, cont := range row {
			c.ioOutput <- cont
		}
	}
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
func cellAlive(y, x int, world [][]byte) int {
	if world[y][x] == Alive {
		return 1
	} else {
		return 0
	}
}

// aliveNeighbours calculates the number of alive neighbours for a given cell
func aliveNeighbours(y, x, height, width int, world [][]byte) int {
	// Calculates indices of neighbours (wrapping corner if needed)
	below := (y + 1) % height
	above := y - 1
	if above < 0 {
		above = height - 1
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
func nextState(world, newWorld [][]byte, chunk WorldChunk, turn int, events chan<- Event, chunkUpdated chan<- bool,
	gridHeight, gridWidth int) {
	// Loops through cell's and applies GoL rules to calculate new state
	for y := chunk.StartY; y < chunk.StartY+chunk.RowsProcessed; y++ {
		for x := 0; x < gridWidth; x++ {
			cellCont := world[y][x]
			neighboursAlive := aliveNeighbours(y, x, gridHeight, gridWidth, world)
			// Updates new world cell in chunk based on calculation
			if cellCont == Alive && neighboursAlive < 2 {
				newWorld[y][x] = Dead
			} else if cellCont == Alive && neighboursAlive > 3 {
				newWorld[y][x] = Dead
			} else if cellCont == Dead && neighboursAlive == 3 {
				newWorld[y][x] = Alive
			} else {
				newWorld[y][x] = cellCont
			}
			// Sends cell flipped notification
			if newWorld[y][x] != cellCont {
				events <- CellFlipped{Cell: util.Cell{X: x, Y: y}, CompletedTurns: turn}
			}
		}
	}
	chunkUpdated <- true
}

// noAliveCells calculates number of alive cells
func noAliveCells(world [][]byte) int {
	alive := 0
	// Loops through all cells and calculates number of alive cells
	for _, row := range world {
		for _, cell := range row {
			if cell == Alive {
				alive++
			}
		}
	}
	return alive
}

// Calculates all alive cells in GoL world and returns a list of all alive cells.
func aliveCells(world [][]byte) []util.Cell {
	i := 0
	alive := make([]util.Cell, noAliveCells(world))
	// Loops through all cells and collates list of alive cells
	for y, row := range world {
		for x, cell := range row {
			if cell == Alive {
				alive[i] = util.Cell{X: x, Y: y}
				i++
			}
		}
	}
	return alive
}
