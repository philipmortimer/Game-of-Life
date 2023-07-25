package stubs

/* RPC messages that broker can send to aws worker */

const EvolveGrid = "OperationsServer.EvolveGrid"
const KillServer = "OperationsServer.KillServer"

// KillServerRequest used to indicate that server should be killed
type KillServerRequest struct{}

// KillServerResponse is response type for killing server
type KillServerResponse struct{}

// EvolveGridRequest requests that server evolves grid by single turn
type EvolveGridRequest struct {
	AboveRow  []byte
	BelowRow  []byte
	Grid      [][]byte
	Nothreads int
}

// EvolveGridResponse returns evolved grid
type EvolveGridResponse struct {
	Grid [][]byte
}
