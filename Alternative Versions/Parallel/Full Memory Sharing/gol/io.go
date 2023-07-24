package gol

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

type ioChannels struct {
	CommandBeingExecuted *sync.Mutex
	IoInput              *IoGrid
	IoOutput             *IoGrid
}

// ioState is the internal ioState of the io goroutine.
type ioState struct {
	params   Params
	channels ioChannels
}

// ioCommand allows requesting behaviour from the io (pgm) goroutine.
type ioCommand uint8

// This is a way of creating enums in Go.
// It will evaluate to:
//		ioOutput 	= 0
//		ioInput 	= 1
//		ioCheckIdle = 2
const (
	ioOutput ioCommand = iota
	ioInput
	ioCheckIdle
)

// writePgmImage receives an array of bytes and writes it to a pgm file.
func (io *ioState) writePgmImage() {
	_ = os.Mkdir("out", os.ModePerm)

	// Request a filename from the distributor.
	filename := io.channels.IoOutput.FileName

	file, ioError := os.Create("out/" + filename + ".pgm")
	util.Check(ioError)
	defer file.Close()

	_, _ = file.WriteString("P5\n")
	//_, _ = file.WriteString("# PGM file writer by pnmmodules (https://github.com/owainkenwayucl/pnmmodules).\n")
	_, _ = file.WriteString(strconv.Itoa(io.params.ImageWidth))
	_, _ = file.WriteString(" ")
	_, _ = file.WriteString(strconv.Itoa(io.params.ImageHeight))
	_, _ = file.WriteString("\n")
	_, _ = file.WriteString(strconv.Itoa(255))
	_, _ = file.WriteString("\n")

	for y := 0; y < io.params.ImageHeight; y++ {
		for x := 0; x < io.params.ImageWidth; x++ {
			_, ioError = file.Write([]byte{io.channels.IoOutput.Grid[y][x]})
			util.Check(ioError)
		}
	}

	ioError = file.Sync()
	util.Check(ioError)

	fmt.Println("File", filename, "output done!")
}

// readPgmImage opens a pgm file and sends its data as an array of bytes.
func (io *ioState) readPgmImage() {

	// Request a filename from the distributor.
	filename := io.channels.IoInput.FileName

	data, ioError := ioutil.ReadFile("images/" + filename + ".pgm")
	util.Check(ioError)

	fields := strings.Fields(string(data))

	if fields[0] != "P5" {
		panic("Not a pgm file")
	}

	width, _ := strconv.Atoi(fields[1])
	if width != io.params.ImageWidth {
		panic("Incorrect width")
	}

	height, _ := strconv.Atoi(fields[2])
	if height != io.params.ImageHeight {
		panic("Incorrect height")
	}

	maxval, _ := strconv.Atoi(fields[3])
	if maxval != 255 {
		panic("Incorrect maxval/bit depth")
	}

	image := []byte(fields[4])
	y := 0
	x := 0
	for _, b := range image {
		io.channels.IoInput.Grid[y][x] = b
		x = (x + 1) % io.params.ImageWidth
		if x == 0 {
			y++
		}
	}

	fmt.Println("File", filename, "input done!")
}

// handleIoInput handles input to io in loop
func (io *ioState) handleIoInput() {
	for {
		io.channels.IoInput.CommandSend.Wait()
		io.channels.CommandBeingExecuted.Lock()
		io.readPgmImage()
		io.channels.IoInput.CommandReturn.Post()
		io.channels.CommandBeingExecuted.Unlock()
	}
}

// Handles Io output commands as they come
func (io *ioState) handleIoOutput() {
	for {
		io.channels.IoOutput.CommandSend.Wait()
		io.channels.CommandBeingExecuted.Lock()
		io.writePgmImage()
		io.channels.IoOutput.CommandReturn.Post()
		io.channels.CommandBeingExecuted.Unlock()
	}
}

// startIo should be the entrypoint of the io goroutine.
func startIo(p Params, c ioChannels) {
	io := ioState{
		params:   p,
		channels: c,
	}

	go io.handleIoInput()
	go io.handleIoOutput()
}
