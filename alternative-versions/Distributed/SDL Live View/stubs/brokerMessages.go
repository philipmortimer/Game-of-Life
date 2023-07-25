package stubs

import "uk.ac.bris.cs/gameoflife/util"

/* RPC messages that controller can send to broker */

const ProcessGol = "OperationsBroker.ProcessGol"
const GetBoardState = "OperationsBroker.GetBoardState"
const GetNoAliveCells = "OperationsBroker.GetNoAliveCells"
const InitialiseBroker = "OperationsBroker.InitialiseBroker"
const DistributorQuit = "OperationsBroker.DistributorQuit"
const KillDistributed = "OperationsBroker.KillDistributed"
const SetPaused = "OperationsBroker.SetPaused"

const BrokerAddress = "127.0.0.1:8031"

// SetPausedRequest requests the distributor to pause / unpause
type SetPausedRequest struct {
	Paused bool
}

// SetPausedResponse response to pause request
type SetPausedResponse struct {
	Message string
}

// KillDistributedRequest used to indicate that 'k' has been pressed
type KillDistributedRequest struct{}

// KillDistributedResponse is response type for killing broker
type KillDistributedResponse struct{}

// DistributorQuitRequest used to indicate that distributor is quitting (i.e. 'q' pressed)
type DistributorQuitRequest struct{}

// DistributorQuitResponse is response type for quiting distributor
type DistributorQuitResponse struct{}

// InitialiseBrokerRequest tells broker to initialise data structures to start computation
type InitialiseBrokerRequest struct {
	StartGrid [][]byte
	Turns     int
}

// InitialiseBrokerResponse returns once data has been initialised
type InitialiseBrokerResponse struct{}

// GetBoardRequest requests to get the current turn and number of alive cells
type GetBoardRequest struct{}

// GetBoardResponse returns the current state of the board
type GetBoardResponse struct {
	AliveCells     []util.Cell
	TurnsCompleted int
}

// GetNoAliveCellsRequest used to request the number of alive cells in game
type GetNoAliveCellsRequest struct{}

// GetNoAliveCellsResponse returns number of alive cells
type GetNoAliveCellsResponse struct {
	AliveCells     int
	TurnsCompleted int
}

// ProcessGolRequest is the request to start calculate GoL on the grid
type ProcessGolRequest struct{}

// ProcessGolResponse returns the processed GoL grid after the specified number of turns
type ProcessGolResponse struct {
	AliveCells     []util.Cell
	TurnsCompleted int
}
