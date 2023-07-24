package gol

import (
	"fmt"
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
const BrokerAddress = "127.0.0.1:8030"

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// getGrid gets the board from io
func getGrid(p Params, c distributorChannels) [][]byte {
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
	c distributorChannels, quit *bool, aliveCells *[]util.Cell, turnsCompleted *int, gridRpc chan *rpc.Call) {
	if key == 's' && !*paused { // Saves current grid to image
		resp := new(stubs.GetBoardResponse)
		check(client.Call(stubs.GetBoardState, stubs.GetBoardRequest{}, resp))
		writeImage(resp.AliveCells, c, p, resp.TurnsCompleted)
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
func processLoop(finalGridResp *stubs.ProcessGolResponse, client *rpc.Client, p Params, c distributorChannels,
	aliveCells *[]util.Cell, turnsCompleted *int, gridRpc chan *rpc.Call, ticker *time.Ticker, keyPresses <-chan rune) {
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
			handleKeyPress(key, &paused, finalGridResp, client, p, c, &quit, aliveCells, turnsCompleted, gridRpc)
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	client, err := rpc.Dial("tcp", BrokerAddress) // Connects to broker
	check(err)
	// error handling wrapped in a closure
	defer func(client *rpc.Client) {
		errClosing := client.Close()
		check(errClosing)
	}(client)
	// Makes call to start processing
	gridRpc := make(chan *rpc.Call, 1)
	err = client.Call(stubs.InitialiseBroker,
		stubs.InitialiseBrokerRequest{StartGrid: getGrid(p, c),
			Turns: p.Turns, Threads: p.Threads}, new(stubs.InitialiseBrokerResponse))
	check(err)
	// Stars main GoL computation
	finalGridResp := new(stubs.ProcessGolResponse)
	client.Go(stubs.ProcessGol, stubs.ProcessGolRequest{}, finalGridResp, gridRpc)
	ticker := time.NewTicker(2 * time.Second) // Ticker used to send list of alive cells every 2 seconds
	var aliveCells []util.Cell
	var turnsCompleted int
	processLoop(finalGridResp, client, p, c, &aliveCells, &turnsCompleted, gridRpc, ticker, keyPresses)
	ticker.Stop()
	writeImage(aliveCells, c, p, turnsCompleted)
	gameFinished(c, p.Turns, aliveCells)
}

// use a broker this will distribute all the work to different servers

// writeImage writes the provided GoL board as PGM file
func writeImage(aliveCells []util.Cell, c distributorChannels, p Params, turn int) {
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
func gameFinished(c distributorChannels, turn int, aliveCells []util.Cell) {
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	// Reports the final state and quits
	c.events <- FinalTurnComplete{turn, aliveCells}
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
