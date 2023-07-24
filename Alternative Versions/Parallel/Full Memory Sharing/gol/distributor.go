package gol

import (
	"fmt"
	"github.com/ChrisGora/semaphore"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	IoChannels ioChannels
	events     *EventHandler
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

// WorkerDistSync Struct used for global variable that enables synchronisation between workers and distributor without
// using channels.
type WorkerDistSync struct {
	NoWorkers           int
	NoWorkersFinished   int
	AllWorkersFinished  semaphore.Semaphore
	WriteWorkerFinished sync.Mutex
}

type LoopState struct {
	algoData GameOfLifeData
	lock     sync.Mutex
	pause    bool
	quit     bool
	turn     int
}

var state LoopState

var workerStatus WorkerDistSync

const Alive byte = 255
const Dead byte = 0

// handleKeyPresses Repeatedly handles key presses
func handleKeyPresses(keyPresses *KeyHandler, p Params, c distributorChannels) {
	if keyPresses == nil {
		return
	}
	for {
		keyPresses.KeyPressReceived.Wait()
		key := keyPresses.KeyPressed
		state.lock.Lock()
		handleKeyPress(key, p, c)
		state.lock.Unlock()
		keyPresses.KeyPressHandled.Post()
	}
}

func continueLoop(p Params) bool {
	state.lock.Lock()
	cont := state.turn < p.Turns && !state.quit
	state.lock.Unlock()
	return cont
}

func isPaused() bool {
	state.lock.Lock()
	p := state.pause
	state.lock.Unlock()
	return p
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses *KeyHandler) {
	// Two world structures are used to store the current and next boards
	state.algoData = getGolData(p, c)
	initSyncData(len(state.algoData.ThreadChunks))
	ticker := time.NewTicker(2 * time.Second) // Ticker used to send list of alive cells every 2 seconds*/
	// Executes all turns of the Game of Life. Also handles various other events
	state.lock.Lock()
	state.turn = 0
	state.pause = false
	state.quit = false
	state.lock.Unlock()
	go handleKeyPresses(keyPresses, p, c)
	for continueLoop(p) {
		select {
		case <-ticker.C:
			if !isPaused() { // Sends number of alive cells
				c.events.SendEvent(AliveCellsCount{CellsCount: noAliveCells(state.algoData.CurrentGrid), CompletedTurns: state.turn})
			}
		default:
			if !isPaused() { // By default, will evolve board state by one turn and update grids
				evolveGrid(c.events)
			}
		}
	}
	writeImage(state.algoData.CurrentGrid, c, p, state.turn) // Outputs pgm image after all turns completed
	ticker.Stop()                                            // Stops ticker
	gameFinished(c, state.turn, state.algoData.CurrentGrid)  // Ensures GoL finish occurs correctly
}

// initSyncData sets the data used to sync between distributor and workers
func initSyncData(noProcesses int) {
	workerStatus.NoWorkersFinished = 0
	workerStatus.NoWorkers = noProcesses
	workerStatus.AllWorkersFinished = semaphore.Init(1, 0)
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

// Returns a copy of a matrix
func copyMatrix(grid [][]byte) [][]byte {
	newGrid := makeTwoDMatrix(len(grid), len(grid[0]))
	for y := range grid {
		copy(newGrid[y], grid[y])
	}
	return newGrid
}

// getWorldGrid gets the inputted grid from io
func getWorldGrid(p Params, c distributorChannels) [][]byte {
	// Tells IO to read appropriate image in
	c.IoChannels.IoInput.FileName = strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.IoChannels.IoInput.CommandSend.Post()
	c.IoChannels.IoInput.CommandReturn.Wait()
	world := copyMatrix(c.IoChannels.IoInput.Grid)
	// Reads image into slice
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == Alive {
				c.events.FlipCell(util.Cell{X: x, Y: y})
			}
		}
	}
	return world
}

// handleKeyPress handles key presses made by user in SDL window. If 's' is pressed, game state is saved.
// 'q' saves an image and quits the main game loop. 'p' pauses / unpauses the game. The quit and paused
// boolean variables are updated accordingly.
func handleKeyPress(charPressed rune, p Params, c distributorChannels) {
	if charPressed == 's' {
		writeImage(state.algoData.CurrentGrid, c, p, state.turn) // Saves image
	} else if charPressed == 'p' {
		// Pauses or unpauses game
		state.pause = !state.pause
		newState := Paused
		if !state.pause {
			newState = Executing
			fmt.Println("Continuing") // Print statements look ugly but seem to be required by CW spec
		}
		c.events.SendEvent(StateChange{CompletedTurns: state.turn, NewState: newState})
	} else if charPressed == 'q' {
		state.quit = true
	}
}

// gameFinished gracefully exits game by ensuring all IO process are done and that
// final turn being completed is reported.
func gameFinished(c distributorChannels, turn int, world [][]byte) {
	// Make sure that the Io has finished any output before exiting.
	c.IoChannels.CommandBeingExecuted.Lock()
	c.IoChannels.CommandBeingExecuted.Unlock()
	// Reports the final state and quits
	c.events.SendEvent(FinalTurnComplete{turn, aliveCells(world)})
	c.events.SendEvent(StateChange{turn, Quitting})
}

// evolveGrid evolves GoL grid by 1 turn
func evolveGrid(events *EventHandler) {
	grids := &state.algoData
	// Starts each of worker threads
	workerStatus.WriteWorkerFinished.Lock()
	workerStatus.NoWorkersFinished = 0
	workerStatus.WriteWorkerFinished.Unlock()
	for i := range grids.ThreadChunks {
		go nextState(grids.CurrentGrid, grids.NewGrid, grids.ThreadChunks[i], state.turn, events,
			grids.GridHeight, grids.GridWidth)
	}
	// Ensures that all chunks board data has been updated
	workerStatus.AllWorkersFinished.Wait()
	// Updates variables used in main GoL loop
	state.lock.Lock()
	state.turn++
	events.SendEvent(TurnComplete{state.turn})
	// Updates grids
	oldCurr := (*grids).CurrentGrid
	(*grids).CurrentGrid = (grids).NewGrid
	(*grids).NewGrid = oldCurr
	state.lock.Unlock()
}

// writeImage writes the provided GoL board as PGM file
func writeImage(world [][]byte, c distributorChannels, p Params, turn int) {
	c.IoChannels.IoOutput.FileName =
		strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	// Writes grid to channel
	for y := range world {
		for x := range world[y] {
			c.IoChannels.IoOutput.Grid[y][x] = world[y][x]
		}
	}
	c.IoChannels.IoOutput.CommandSend.Post()
	c.IoChannels.IoOutput.CommandReturn.Wait()
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
func nextState(world, newWorld [][]byte, chunk WorldChunk, turn int, events *EventHandler, gridHeight, gridWidth int) {
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
				events.FlipCell(util.Cell{X: x, Y: y})
			}
		}
	}
	// Signals that worker has finished
	workerStatus.WriteWorkerFinished.Lock()
	workerStatus.NoWorkersFinished++
	allWorkersDone := workerStatus.NoWorkersFinished == workerStatus.NoWorkers
	workerStatus.WriteWorkerFinished.Unlock()
	if allWorkersDone {
		workerStatus.AllWorkersFinished.Post()
	}
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
