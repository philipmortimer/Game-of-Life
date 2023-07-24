package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
	"sync"
	"uk.ac.bris.cs/gameoflife/stubs"
)

const Alive byte = 255
const Dead byte = 0

type OperationsServer struct{}

type ghostRowAbove struct {
	aboverow []byte
	lock     sync.Mutex
}
type ghostRowBelow struct {
	belowrow []byte
	lock     sync.Mutex
}
type Grid struct {
	grid [][]byte
	lock sync.Mutex
}

var aboveRow ghostRowAbove
var belowRow ghostRowBelow
var g Grid

// KillServer kills the server using os.exit
func (s *OperationsServer) KillServer(req stubs.KillDistributedRequest, response *stubs.KillServerResponse) (err error) {
	defer os.Exit(0)
	return nil
}

// EvolveGrid evolves the grid provided by one generation
func (s *OperationsServer) EvolveGrid(req stubs.EvolveGridRequest, response *stubs.EvolveGridResponse) (err error) {
	height := len(req.Grid)
	width := len(req.Grid[0])

	initServerdata(req)
	workers := req.Nothreads
	//if the request did not send the number of threads, default set to 1
	if workers == 0 {
		workers = 1
	}
	// a list of worker and the rows they should be processing
	workload := divideGridForProcessing(workers, height)
	chunks := make([]chan [][]byte, len(workload))
	startY := 0
	for i := 0; i < len(workload); i++ {
		//go work
		chunks[i] = make(chan [][]byte)
		go worker(startY, startY+workload[i], width, chunks[i])
		startY = startY + workload[i]
	}
	for i := 0; i < len(workload); i++ {
		chunk := <-chunks[i]
		response.Grid = append(response.Grid, chunk...)
	}

	return nil
}
func initServerdata(req stubs.EvolveGridRequest) {
	aboveRow.aboverow = req.AboveRow
	belowRow.belowrow = req.BelowRow
	g.grid = req.Grid
}

// worker function that starts the calculation of game of life
func worker(startY, endY, width int, out chan [][]byte) {
	workerChunk := makeTwoDMatrix(endY-startY, width)
	evolveChunk(startY, endY, workerChunk)
	out <- workerChunk
}

func evolveChunk(startY, endY int, newGrid [][]byte) {
	// Main GoL logic
	height := len(g.grid)
	width := len(g.grid[0])
	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			cellCont := g.grid[y][x]
			neighboursAlive := aliveNeighbours(y, x, height, width)
			// Updates new world cell in chunk based on calculation
			if cellCont == Alive && neighboursAlive < 2 {
				newGrid[y-startY][x] = Dead
			} else if cellCont == Alive && neighboursAlive > 3 {
				newGrid[y-startY][x] = Dead
			} else if cellCont == Dead && neighboursAlive == 3 {
				newGrid[y-startY][x] = Alive
			} else {
				newGrid[y-startY][x] = cellCont
			}
		}
	}
}

// divideGridForProcessing divides grid into roughly even chunks that can be processed by each worker thread
func divideGridForProcessing(workers, height int) []int {
	// Divides board evenly amongst thread by giving each thread roughly the same number of rows to process
	rowsPerServer := height / workers
	// Accounts for when number of rows don't nicely divide between number of threads
	if rowsPerServer == 0 { // If there are more threads than rows, some threads don't do any processing.
		rowsPerServer = 1
	}
	noProcessesNeeded := height / rowsPerServer // Works out number of threads actually used
	extraRows := height % rowsPerServer
	// Initialises WorldChunk data structures
	chunks := make([]int, noProcessesNeeded)
	totalRowsAssigned := 0 // Tracks number of rows actually assigned already for processing
	for chunkIndex := 0; chunkIndex < noProcessesNeeded; chunkIndex++ {
		rowsToProcess := rowsPerServer
		if chunkIndex < extraRows {
			rowsToProcess++
		}
		chunks[chunkIndex] = rowsToProcess
		totalRowsAssigned += rowsToProcess
	}
	return chunks
}

// Makes two d matrix of specified size
func makeTwoDMatrix(height, width int) [][]byte {
	x := make([][]byte, height)
	for i := range x {
		x[i] = make([]byte, width)
	}
	return x
}

// aliveNeighbours calculates the number of alive neighbours for a given cell
func aliveNeighbours(y, x, height, width int) int {
	// Calculates indices of neighbours (wrapping corner if needed)
	below := y + 1
	above := y - 1
	right := (x + 1) % width
	left := x - 1
	if left < 0 {
		left = width - 1
	}
	// Calculates alive neighbors
	alive := cellAlive(y, left, height) + cellAlive(above, left, height) +
		cellAlive(below, left, height) + cellAlive(y, right, height) +
		cellAlive(above, right, height) + cellAlive(below, right, height) +
		cellAlive(above, x, height) + cellAlive(below, x, height)
	return alive
}

// cellAlive returns 1 if cell is alive, 0 otherwise
func cellAlive(y, x, height int) int {
	if y >= height {
		if belowRow.belowrow[x] == Alive {
			return 1
		} else {
			return 0
		}
	} else if y < 0 {
		if aboveRow.aboverow[x] == Alive {
			return 1
		} else {
			return 0
		}
	} else {
		if g.grid[y][x] == Alive {
			return 1
		} else {
			return 0
		}
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	pAddr := flag.String("port", "8039", "Port to listen on")
	flag.Parse()
	// use the net/rpc package to register service
	err := rpc.Register(&OperationsServer{})
	check(err)
	listener, err := net.Listen("tcp", ":"+*pAddr)
	check(err)
	//defer means delay the execution of a function or a statement until the nearby function returns.
	//In simple words, defer will move the execution of the statement to the very end inside a function.
	defer listener.Close()
	// we want the service to start accepting the communications
	rpc.Accept(listener)
}
