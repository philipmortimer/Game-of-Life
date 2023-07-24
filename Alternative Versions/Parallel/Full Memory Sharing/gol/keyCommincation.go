package gol

import (
	"github.com/ChrisGora/semaphore"
)

// KeyHandler Used to facilitate pure memory sharing for key presses
type KeyHandler struct {
	KeyPressed       rune
	KeyPressReceived semaphore.Semaphore
	KeyPressHandled  semaphore.Semaphore
}

// InitKeyHandler Initialises a key handler
func InitKeyHandler() *KeyHandler {
	return &KeyHandler{
		KeyPressed:       0,
		KeyPressReceived: semaphore.Init(1, 0),
		KeyPressHandled:  semaphore.Init(1, 1),
	}
}

// SendKey sends a key to be processed
func (k *KeyHandler) SendKey(key rune) {
	k.KeyPressHandled.Wait()
	k.KeyPressed = key
	k.KeyPressReceived.Post()
}
