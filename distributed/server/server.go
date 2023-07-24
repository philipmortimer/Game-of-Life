package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
	"uk.ac.bris.cs/gameoflife/stubs"
)

const Alive byte = 255
const Dead byte = 0

type OperationsServer struct{}

// KillServer kills the server using os.exit
func (s *OperationsServer) KillServer(req stubs.KillDistributedRequest, response *stubs.KillServerResponse) (err error) {
	defer os.Exit(0)
	return nil
}

// EvolveGrid evolves the grid provided by one generation
func (s *OperationsServer) EvolveGrid(req stubs.EvolveGridRequest, response *stubs.EvolveGridResponse) (err error) {
	// Main GoL logic
	height := len(req.Grid)
	width := len(req.Grid[0])
	response.Grid = makeTwoDMatrix(height, width)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cellCont := req.Grid[y][x]
			neighboursAlive := aliveNeighbours(y, x, height, width, req)
			// Updates new world cell in chunk based on calculation
			if cellCont == Alive && neighboursAlive < 2 {
				response.Grid[y][x] = Dead
			} else if cellCont == Alive && neighboursAlive > 3 {
				response.Grid[y][x] = Dead
			} else if cellCont == Dead && neighboursAlive == 3 {
				response.Grid[y][x] = Alive
			} else {
				response.Grid[y][x] = cellCont
			}
		}
	}
	return nil
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
func aliveNeighbours(y, x, height, width int, req stubs.EvolveGridRequest) int {
	// Calculates indices of neighbours (wrapping corner if needed)
	below := y + 1
	above := y - 1
	right := (x + 1) % width
	left := x - 1
	if left < 0 {
		left = width - 1
	}
	// Calculates alive neighbors
	alive := cellAlive(y, left, height, req) + cellAlive(above, left, height, req) +
		cellAlive(below, left, height, req) + cellAlive(y, right, height, req) +
		cellAlive(above, right, height, req) + cellAlive(below, right, height, req) +
		cellAlive(above, x, height, req) + cellAlive(below, x, height, req)
	return alive
}

// cellAlive returns 1 if cell is alive, 0 otherwise
func cellAlive(y, x, height int, req stubs.EvolveGridRequest) int {
	if y >= height {
		if req.BelowRow[x] == Alive {
			return 1
		} else {
			return 0
		}
	} else if y < 0 {
		if req.AboveRow[x] == Alive {
			return 1
		} else {
			return 0
		}
	} else {
		if req.Grid[y][x] == Alive {
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
