// Package gamecontrol defines the GameController interface gameops host
// uses to query status and drive start/stop/restart for any configured game
// instance, without knowing that instance's underlying protocol (RCON for
// Minecraft today, a future game's own REST API tomorrow).
package gamecontrol

import "context"

// Status is a game-agnostic snapshot of one instance.
type Status struct {
	Online      bool
	PlayerCount int
	MaxPlayers  int
	UptimeSec   int64
}

// GameController is the extension point for adding a new game to gameops
// host: implement these four methods against that game's real protocol and
// host/gateway need no other game-specific code.
type GameController interface {
	Status(ctx context.Context) (Status, error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
}
