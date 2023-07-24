package main

import (
	"bytes"
	"flag"
	"github.com/ChrisGora/semaphore"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type OperationsBroker struct{}

const Alive byte = 255

// WorldChunk references a subset of a world. Used to divide number of rows between processes for GoL algo
type WorldChunk struct {
	GridChunk      stubs.EvolveGridRequest
	ServerResponse *stubs.EvolveGridResponse
}

// GameOfLifeData structure stores the structures needed to process GoL in parallel.
type GameOfLifeData struct {
	ServerChunks    []WorldChunk
	ChunksProcessed chan *rpc.Call
}

// BrokerState stores various global variables used to store the state of processing
type BrokerState struct {
	Workers           []*rpc.Client // List of workers
	ConnectedToServer bool          // Indicates whether servers have been connected to
	Data              GameOfLifeData
	Turn              int // The current turn number
	TurnsToProcess    int
	Lock              sync.Mutex // Used to regulate access to world, including evolving data
	Created           bool       // Used to indicate that structure has been initialized at least once
	// ConstLock regulates access to items that remain constant after initialisation.
	// This covers things like StartGrid, Workers and Data.ChunksProcessed. If the main lock exists,
	// safe access to these items should be guaranteed anyway.
	ConstLock sync.Mutex
	StartGrid [][]byte
}

var attemptToReuseComputation bool // Flag to indicate whether fault tolerance mechanism should be enabled

// KeyPressState collection of boolean flags used to indicate state of system (i.e. if it's paused, quitting etc)
type KeyPressState struct {
	Quit     bool
	Lock     sync.Mutex
	PauseSem semaphore.Semaphore
}

var ServerAddresses []string // Stores server addresses
var k KeyPressState
var s BrokerState

// SetPaused pauses / unpauses processing as appropriate
func (b *OperationsBroker) SetPaused(req stubs.SetPausedRequest, response *stubs.SetPausedResponse) (err error) {
	if req.Paused {
		k.PauseSem.Wait()
		response.Message = "Paused    Turn Being Processed " + strconv.Itoa(s.Turn)
	} else {
		k.PauseSem.Post()
		response.Message = "Continuing"
	}
	return nil
}

// KillDistributed kills the connections and the server. Only should be called when no other processes
// are going on for broker or servers
func (b *OperationsBroker) KillDistributed(req stubs.KillDistributedRequest, response *stubs.KillDistributedResponse) (err error) {
	s.Lock.Lock()
	for _, w := range s.Workers {
		w.Go(stubs.KillServer, stubs.KillServerRequest{}, new(stubs.KillServerResponse), nil)
	}
	s.Lock.Unlock()
	defer os.Exit(0)
	return nil
}

// DistributorQuit used to indicate to broker that distributor is quitting
func (b *OperationsBroker) DistributorQuit(req stubs.DistributorQuitRequest, response *stubs.DistributorQuitResponse) (err error) {
	k.Lock.Lock()
	k.Quit = true
	k.Lock.Unlock()
	return nil
}

// GetNoAliveCells gets the number of alive cells in the current grid
func (b *OperationsBroker) GetNoAliveCells(req stubs.GetNoAliveCellsRequest, response *stubs.GetNoAliveCellsResponse) (err error) {
	s.Lock.Lock()
	response.AliveCells = noAliveCells()
	response.TurnsCompleted = s.Turn
	s.Lock.Unlock()
	return nil
}

// GetBoardState gets the state of the current grid
func (b *OperationsBroker) GetBoardState(req stubs.GetBoardRequest, response *stubs.GetBoardResponse) (err error) {
	s.ConstLock.Lock()
	s.Lock.Lock()
	response.AliveCells = getAliveCells()
	response.TurnsCompleted = s.Turn
	s.Lock.Unlock()
	s.ConstLock.Unlock()
	return nil
}

// InitialiseBroker initializes the broker with the provided parameters.
// Once initialized, the broker can start computation.
func (b *OperationsBroker) InitialiseBroker(req stubs.InitialiseBrokerRequest, response *stubs.InitialiseBrokerResponse) (err error) {
	// Inits game state
	s.Lock.Lock()
	s.ConstLock.Lock()
	initGameData(req.StartGrid, req.Turns)
	s.ConstLock.Unlock()
	s.Lock.Unlock()
	// Inits key presses state
	k.Lock.Lock()
	initKeyPress()
	k.Lock.Unlock()
	return nil
}

// turnsStillToProcess checks to see that there are still turns to process.
// Does this using a lock
func turnsStillToProcess() bool {
	s.Lock.Lock()
	ret := s.Turn < s.TurnsToProcess
	s.Lock.Unlock()
	return ret
}

// checkQuit checks to see if quit variable is false using lock
func checkQuit() bool {
	k.Lock.Lock()
	ret := k.Quit
	k.Lock.Unlock()
	return ret
}

// updateWorld updates the grid values following server response
func updateWorld() {
	// Updates world
	k.PauseSem.Wait()
	s.Lock.Lock()
	for i := range s.Workers {
		for row := range s.Data.ServerChunks[i].GridChunk.Grid {
			copy(s.Data.ServerChunks[i].GridChunk.Grid[row], s.Data.ServerChunks[i].ServerResponse.Grid[row])
		}
	}
	s.Turn++
	k.PauseSem.Post()
	s.Lock.Unlock()
}

// ProcessGol runs GoL for the number of iterations on the grid.
// Make sure to initialise broker before calling this method
func (b *OperationsBroker) ProcessGol(req stubs.ProcessGolRequest, response *stubs.ProcessGolResponse) (err error) {
	for turnsStillToProcess() && !checkQuit() {
		//Sets ghost rows
		s.Lock.Lock()
		setGhostRows()
		// Sends work to nodes to evolve grid
		for i := range s.Workers {
			s.Workers[i].Go(stubs.EvolveGrid, s.Data.ServerChunks[i].GridChunk,
				s.Data.ServerChunks[i].ServerResponse, s.Data.ChunksProcessed)
		}
		s.Lock.Unlock()
		// Waits for every node's response
		s.ConstLock.Lock()
		for range s.Workers {
			dat := <-s.Data.ChunksProcessed
			// Checks for RPC error
			if dat.Error != nil {
				closeConnections()
				return err
			}
		}
		s.ConstLock.Unlock()
		updateWorld() // Updates world
	}
	s.Lock.Lock()
	response.AliveCells = getAliveCells()
	response.TurnsCompleted = s.Turn
	s.Lock.Unlock()
	return nil
}

// noAliveCells gets the number of alive cells
func noAliveCells() int {
	grid := s.Data.ServerChunks
	alive := 0
	for i := 0; i < len(grid); i++ {
		for y := 0; y < len(grid[i].GridChunk.Grid); y++ {
			for x := 0; x < len(grid[i].GridChunk.Grid[0]); x++ {
				if grid[i].GridChunk.Grid[y][x] == Alive {
					alive++
				}
			}
		}
	}
	return alive
}

// getAliveCells gets the list of alive cells
func getAliveCells() []util.Cell {
	grid := s.Data.ServerChunks
	alive := make([]util.Cell, noAliveCells())
	aliveIndex := 0
	rowsProcessed := 0
	for i := 0; i < len(grid); i++ {
		for y := 0; y < len(grid[i].GridChunk.Grid); y++ {
			for x := 0; x < len(grid[i].GridChunk.Grid[0]); x++ {
				if grid[i].GridChunk.Grid[y][x] == Alive {
					alive[aliveIndex] = util.Cell{X: x, Y: y + rowsProcessed}
					aliveIndex++
				}
			}
		}
		rowsProcessed += len(grid[i].GridChunk.Grid)
	}
	return alive
}

// closeConnections closes rpc connections
func closeConnections() {
	for _, conn := range s.Workers {
		conn.Close()
	}
}

// initKeyPress initialises key presses flag to starting states. Lock should be acquired before calling this
func initKeyPress() {
	k.Quit = false
	if k.PauseSem.GetValue() == 0 {
		k.PauseSem.Post()
	}
}

// initGameData initialises the game of life structure
func initGameData(startGrid [][]byte, turnsToProcess int) {
	connectToServers()
	turnsPrev := s.Turn
	s.TurnsToProcess = turnsToProcess
	// Checks to see if board already exists from a previous controller and continues processing
	// This is a form of fault tolerance
	if attemptToReuseComputation && s.Created && (len(startGrid) == len(s.StartGrid)) &&
		(len(startGrid[0]) == len(s.StartGrid[0])) && (turnsPrev < turnsToProcess) {
		sameGrid := true
		for i := range startGrid {
			sameGrid = sameGrid && bytes.Equal(startGrid[i], s.StartGrid[i])
		}
		if sameGrid {
			s.Turn = turnsPrev
			return
		}
	}
	s.Turn = 0
	s.Created = true
	s.StartGrid = startGrid
	rowsPerServer := divideGridForProcessing(len(s.Workers), len(startGrid))
	chunksCurr := generateChunks(rowsPerServer, startGrid) // Creates chunks
	s.Data = GameOfLifeData{ServerChunks: chunksCurr,
		ChunksProcessed: make(chan *rpc.Call, len(chunksCurr))}
}

// generateChunks splits the grid into their specified chunks. Each chunk stores a section of the GoL board
// (split by rows).
func generateChunks(rowsPerServer []int, startGrid [][]byte) []WorldChunk {
	chunksCurr := make([]WorldChunk, len(rowsPerServer))
	width := len(startGrid[0])
	rowsProcessed := 0
	for i, rows := range rowsPerServer {
		chunksCurr[i] = WorldChunk{GridChunk: stubs.EvolveGridRequest{
			Grid: makeTwoDMatrix(rows, width), BelowRow: make([]byte, width),
			AboveRow: make([]byte, width)},
			ServerResponse: new(stubs.EvolveGridResponse)}
		// Sets grid value for current chunk
		for y := 0; y < rows; y++ {
			copy(chunksCurr[i].GridChunk.Grid[y], startGrid[y+rowsProcessed])
		}
		rowsProcessed += rows
	}
	return chunksCurr
}

// Sets the above and below ghost rows based on the values stored within the grid chunk
func setGhostRows() {
	chunks := s.Data.ServerChunks
	noChunks := len(s.Data.ServerChunks)
	for i := range chunks {
		above := i - 1
		if above < 0 {
			above = noChunks - 1
		}
		below := (i + 1) % noChunks
		copy(s.Data.ServerChunks[i].GridChunk.BelowRow, chunks[below].GridChunk.Grid[0])
		copy(s.Data.ServerChunks[i].GridChunk.AboveRow,
			chunks[above].GridChunk.Grid[len(chunks[above].GridChunk.Grid)-1])
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

// divideGridForProcessing divides grid into roughly even chunks that can be processed by each server
func divideGridForProcessing(servers, height int) []int {
	// Divides board evenly amongst thread by giving each thread roughly the same number of rows to process
	rowsPerServer := height / servers

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

// connectToServers attempts to establish rpc connection with servers.
func connectToServers() error {
	if s.ConnectedToServer {
		return nil
	}
	// Creates connection to each server
	s.Workers = make([]*rpc.Client, 0)
	for _, address := range ServerAddresses {
		client, err := rpc.Dial("tcp", address)
		if err != nil {
			return err
		}
		s.Workers = append(s.Workers, client)
	}
	s.ConnectedToServer = true
	return nil
}

// check handles an error by attempting to close all connections and then panicking.
func check(e error) {
	if e != nil {
		closeConnections()
		panic(e)
	}
}

func main() {
	pAddr := flag.String("port", "8029", "Port to listen on")
	serverAdds := flag.String("serverAddressesList",
		"34.207.219.238:8039",
		"List of server addresses as csv")
	reuseBoard := flag.Bool("reuse", false,
		"Indicates whether broker should attempt to use previous computation if it's the same board"+
			". This is a form of fault tolerance")
	flag.Parse()
	attemptToReuseComputation = *reuseBoard
	// Connects to aws servers
	ServerAddresses = strings.Split(*serverAdds, ",")
	s.ConnectedToServer = false
	k.PauseSem = semaphore.Init(1, 1)
	err := connectToServers()
	check(err)
	// use the net/rpc package to register service
	err = rpc.Register(&OperationsBroker{})
	check(err)
	listener, err := net.Listen("tcp", ":"+*pAddr)
	check(err)
	//defer means delay the execution of a function or a statement until the nearby function returns.
	//In simple words, defer will move the execution of the statement to the very end inside a function.
	defer listener.Close()
	// we want the service to start accepting the communications
	rpc.Accept(listener)
}
