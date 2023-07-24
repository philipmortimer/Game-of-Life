package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
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

const Alive = 255

var keyPresses <-chan rune
var c distributorChannels

type OperationsDistributor struct{}

var rpcStarted = false

// startRpc Starts RPC by listening in on appropriate port and registering rpc funcs.
func startRpc() {
	if rpcStarted {
		return
	}
	// use the net/rpc package to register service
	err := rpc.Register(&OperationsDistributor{})
	check(err)
	listener, err := net.Listen("tcp", ":"+stubs.DistributorPort)
	check(err)
	//defer means delay the execution of a function or a statement until the nearby function returns.
	//In simple words, defer will move the execution of the statement to the very end inside a function.
	defer listener.Close()
	// we want the service to start accepting the communications
	rpcStarted = true
	rpc.Accept(listener)
}

// ReportCellsFlipped reports cells that have flipped
func (d *OperationsDistributor) ReportCellsFlipped(req stubs.ReportCellsFlippedRequest,
	response *stubs.ReportCellsFlippedResponse) (err error) {
	for _, cell := range req.CellsFlipped {
		c.events <- CellFlipped{Cell: cell, CompletedTurns: req.TurnsCompleted}
	}
	c.events <- TurnComplete{CompletedTurns: req.TurnsCompleted}
	return nil
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// getGrid gets the board from io
func getGrid(p Params) [][]byte {
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	world := makeTwoDMatrix(p.ImageHeight, p.ImageWidth)
	// Reads image into slice
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}
	return world
}

// handleKillKey is a helper function that helps kill the distributed system when the 'k' key is pressed
func handleKillKey(paused *bool, finalGridResp *stubs.ProcessGolResponse, client *rpc.Client,
	quit *bool, aliveCells *[]util.Cell, turnsCompleted *int, gridRpc chan *rpc.Call) {
	//Unpauses if required
	if *paused {
		*paused = false
		check(client.Call(stubs.SetPaused, stubs.SetPausedRequest{Paused: *paused},
			new(stubs.SetPausedResponse)))
	}
	// Quits server
	check(client.Call(stubs.DistributorQuit,
		stubs.DistributorQuitRequest{}, new(stubs.DistributorQuitResponse)))
	check((<-gridRpc).Error)
	// Sets grid
	*aliveCells = finalGridResp.AliveCells
	*turnsCompleted = finalGridResp.TurnsCompleted
	*quit = true
	// Kills distributed
	client.Go(stubs.KillDistributed, stubs.KillDistributedRequest{},
		new(stubs.KillDistributedResponse), nil)
}

// handleKeyPress handles a key being pressed. Updated relevant variables used in main
// GoL loop as required.
func handleKeyPress(key rune, paused *bool, finalGridResp *stubs.ProcessGolResponse, client *rpc.Client, p Params,
	quit *bool, aliveCells *[]util.Cell, turnsCompleted *int, gridRpc chan *rpc.Call) {
	if key == 's' && !*paused { // Saves current grid to image
		resp := new(stubs.GetBoardResponse)
		check(client.Call(stubs.GetBoardState, stubs.GetBoardRequest{}, resp))
		writeImage(resp.AliveCells, p, resp.TurnsCompleted)
	} else if key == 'p' {
		*paused = !*paused
		resp := new(stubs.SetPausedResponse)
		check(client.Call(stubs.SetPaused, stubs.SetPausedRequest{Paused: *paused}, resp))
		fmt.Println(resp.Message)
	} else if key == 'q' && !*paused {
		check(client.Call(stubs.DistributorQuit,
			stubs.DistributorQuitRequest{}, new(stubs.DistributorQuitResponse)))
		check((<-gridRpc).Error)
		client.Close()
		os.Exit(0)
	} else if key == 'k' {
		handleKillKey(paused, finalGridResp, client, quit, aliveCells, turnsCompleted, gridRpc)
	}
}

// processLoop is a helper function that handles the main processing loop of the distributor
func processLoop(finalGridResp *stubs.ProcessGolResponse, client *rpc.Client, p Params,
	aliveCells *[]util.Cell, turnsCompleted *int, gridRpc chan *rpc.Call, ticker *time.Ticker) {
	quit := false
	paused := false
	for !quit {
		select {
		// Handles function call return
		case res := <-gridRpc:
			if !paused {
				check(res.Error) // Handles error with rpc call
				*aliveCells = finalGridResp.AliveCells
				*turnsCompleted = finalGridResp.TurnsCompleted
				quit = true
			}
		case <-ticker.C:
			if !paused {
				resp := new(stubs.GetNoAliveCellsResponse)
				check(client.Call(stubs.GetNoAliveCells, stubs.GetNoAliveCellsRequest{}, resp))
				c.events <- AliveCellsCount{CellsCount: resp.AliveCells, CompletedTurns: resp.TurnsCompleted}
			}
		// Handles key inputs
		case key := <-keyPresses:
			handleKeyPress(key, &paused, finalGridResp, client, p, &quit, aliveCells, turnsCompleted, gridRpc)
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, chanDist distributorChannels, keyPressChan <-chan rune) {
	// Inits chan and key as global variables
	go startRpc()
	c = chanDist
	keyPresses = keyPressChan
	client, err := rpc.Dial("tcp", stubs.BrokerAddress) // Connects to broker
	check(err)
	// error handling wrapped in a closure
	defer func(client *rpc.Client) {
		errClosing := client.Close()
		check(errClosing)
	}(client)
	// Makes call to start processing
	gridRpc := make(chan *rpc.Call, 1)
	err = client.Call(stubs.InitialiseBroker,
		stubs.InitialiseBrokerRequest{StartGrid: getGrid(p),
			Turns: p.Turns}, new(stubs.InitialiseBrokerResponse))
	check(err)
	// Stars main GoL computation
	finalGridResp := new(stubs.ProcessGolResponse)
	client.Go(stubs.ProcessGol, stubs.ProcessGolRequest{}, finalGridResp, gridRpc)
	ticker := time.NewTicker(2 * time.Second) // Ticker used to send list of alive cells every 2 seconds
	var aliveCells []util.Cell
	var turnsCompleted int
	processLoop(finalGridResp, client, p, &aliveCells, &turnsCompleted, gridRpc, ticker)
	ticker.Stop()
	writeImage(aliveCells, p, turnsCompleted)
	gameFinished(p.Turns, aliveCells)
}

// use a broker this will distribute all the work to different servers

// writeImage writes the provided GoL board as PGM file
func writeImage(aliveCells []util.Cell, p Params, turn int) {
	// Creates board consisting using list of alive cells
	grid := makeTwoDMatrix(p.ImageHeight, p.ImageWidth)
	for _, cell := range aliveCells {
		grid[cell.Y][cell.X] = Alive
	}
	// Writes image to io
	c.ioCommand <- ioOutput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	// Writes grid to channel
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- grid[y][x]
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

// gameFinished will finish the game and report all the events at finished
func gameFinished(turn int, aliveCells []util.Cell) {
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	// Reports the final state and quits
	c.events <- FinalTurnComplete{turn, aliveCells}
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
