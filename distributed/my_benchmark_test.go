package main

import (
	"fmt"
	"os"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
)

const benchLength = 1000

func BenchmarkGol(b *testing.B) {
	for threads := 1; threads <= 1; threads++ {
		os.Stdout = nil // Disable all program output apart from benchmark results
		p := gol.Params{
			Turns:       benchLength,
			Threads:     threads,
			ImageWidth:  512,
			ImageHeight: 512,
		}
		name := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				events := make(chan gol.Event, p.ImageWidth*p.ImageHeight)
				go gol.Run(p, events, nil)
				for range events {

				}
			}
		})
	}
}

//go test -run ^$ -bench . -benchtime 1x -count 10 -timeout 300m | tee results.out
//go run golang.org/x/perf/cmd/benchstat -csv results.out | tee results.csv
//python plot.py
