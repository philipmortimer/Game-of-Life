package stubs

import "uk.ac.bris.cs/gameoflife/util"

/* Defines messages from broker to server */

const ReportCellsFlipped = "OperationsDistributor.ReportCellsFlipped"

const DistributorPort = "8030"
const DistributorAddress = "127.0.0.1:" + DistributorPort

// ReportCellsFlippedRequest requests the distributor to report that a cell has flipped
type ReportCellsFlippedRequest struct {
	CellsFlipped   []util.Cell
	TurnsCompleted int
}

// ReportCellsFlippedResponse response type for reporting cell flipped
type ReportCellsFlippedResponse struct{}
