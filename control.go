package netorcai

import (
	"encoding/json"
	"fmt"
	"github.com/mpoquet/go-prompt"
	log "github.com/sirupsen/logrus"
	"net"
	"sync"
)

// Game state
const (
	GAME_NOT_RUNNING = iota
	GAME_RUNNING     = iota
	GAME_FINISHED    = iota
)

// Client state
const (
	CLIENT_UNLOGGED = iota
	CLIENT_LOGGED   = iota
	CLIENT_READY    = iota
	CLIENT_THINKING = iota
	CLIENT_KICKED   = iota
)

type GlobalState struct {
	Mutex     sync.Mutex
	WaitGroup sync.WaitGroup

	Listener net.Listener
	prompt   *prompt.Prompt

	GameState int

	GameLogic      []*GameLogicClient
	Players        []*PlayerOrVisuClient
	SpecialPlayers []*PlayerOrVisuClient
	Visus          []*PlayerOrVisuClient

	NbPlayersMax                int
	NbSpecialPlayersMax         int
	NbVisusMax                  int
	NbTurnsMax                  int
	Autostart                   bool
	Fast                        bool
	MillisecondsBeforeFirstTurn float64
	MillisecondsBetweenTurns    float64
}

// Debugging helpers
const (
	debugGlobalStateMutex = false
)

func LockGlobalStateMutex(gs *GlobalState, reason, who string) {
	if debugGlobalStateMutex {
		log.WithFields(log.Fields{
			"reason": reason,
			"who":    who,
		}).Info("Desire global state mutex")
	}
	gs.Mutex.Lock()
	if debugGlobalStateMutex {
		log.WithFields(log.Fields{
			"reason": reason,
			"who":    who,
		}).Info("Got global state mutex")
	}
}

func UnlockGlobalStateMutex(gs *GlobalState, reason, who string) {
	if debugGlobalStateMutex {
		log.WithFields(log.Fields{
			"reason": reason,
			"who":    who,
		}).Info("Release global state mutex")
	}
	gs.Mutex.Unlock()
}

func areAllExpectedClientsConnected(gs *GlobalState) bool {
	return (len(gs.Players) == gs.NbPlayersMax) &&
		(len(gs.SpecialPlayers) == gs.NbSpecialPlayersMax) &&
		(len(gs.Visus) == gs.NbVisusMax) &&
		(len(gs.GameLogic) == 1)
}

func autostart(gs *GlobalState) {
	if gs.Autostart && areAllExpectedClientsConnected(gs) {
		log.Info("Automatic starting conditions are met")
		gs.GameState = GAME_RUNNING
		gs.GameLogic[0].start <- 1
	}
}

func handleClient(client *Client, globalState *GlobalState,
	gameLogicExit chan int) {
	log.WithFields(log.Fields{
		"remote address": client.Conn.RemoteAddr(),
	}).Debug("New connection")

	defer globalState.WaitGroup.Done()
	defer client.Conn.Close()
	// This is to send a shutdown on the socket before closing it.
	// Combined with a SO_LINGER<0 (default for go sockets),
	// this should avoid loss of data sent by netorcai on client sockets.
	defer client.Conn.(*net.TCPConn).CloseWrite()

	go readClientMessages(client)

	msg := <-client.incomingMessages
	if msg.err != nil {
		log.WithFields(log.Fields{
			"err":            msg.err,
			"remote address": client.Conn.RemoteAddr(),
		}).Debug("Cannot receive client first message")
		Kick(client, fmt.Sprintf("Invalid first message: %v", msg.err.Error()))
		return
	}

	loginMessage, err := readLoginMessage(msg.content)
	if err != nil {
		log.WithFields(log.Fields{
			"err":            err,
			"remote address": client.Conn.RemoteAddr(),
		}).Debug("Cannot read LOGIN message")
		Kick(client, fmt.Sprintf("Invalid first message: %v", err.Error()))
		return
	}
	client.nickname = loginMessage.nickname

	LockGlobalStateMutex(globalState, "New client", "Login manager")
	switch loginMessage.role {
	case "player", "special player":
		isSpecial := loginMessage.role == "special player"
		if globalState.GameState != GAME_NOT_RUNNING {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: Game has been started")
		} else if !isSpecial && len(globalState.Players) >= globalState.NbPlayersMax {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: Maximum number of players reached")
		} else if isSpecial && len(globalState.SpecialPlayers) >= globalState.NbSpecialPlayersMax {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: Maximum number of special players reached")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				UnlockGlobalStateMutex(globalState, "New client", "Login manager")
				Kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				pvClient := &PlayerOrVisuClient{
					client:          client,
					playerID:        -1,
					isPlayer:        true,
					isSpecialPlayer: isSpecial,
					gameStarts:      make(chan MessageGameStarts),
					newTurn:         make(chan MessageTurn, 100),
					gameEnds:        make(chan MessageGameEnds, 1),
					playerInfo:      nil,
				}

				if !isSpecial {
					globalState.Players = append(globalState.Players, pvClient)
				} else {
					globalState.SpecialPlayers = append(globalState.SpecialPlayers, pvClient)
				}

				log.WithFields(log.Fields{
					"nickname":             client.nickname,
					"remote address":       client.Conn.RemoteAddr(),
					"player count":         len(globalState.Players),
					"special player count": len(globalState.SpecialPlayers),
					"special":              isSpecial,
				}).Info("New player accepted")
				client.state = CLIENT_LOGGED

				UnlockGlobalStateMutex(globalState, "New client", "Login manager")

				// Automatically start the game if conditions are met
				autostart(globalState)

				// Player behavior is handled in dedicated function.
				handlePlayerOrVisu(pvClient, globalState)
			}
		}
	case "visualization":
		if len(globalState.Visus) >= globalState.NbVisusMax {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: Maximum number of visus reached")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				UnlockGlobalStateMutex(globalState, "New client", "Login manager")
				Kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				pvClient := &PlayerOrVisuClient{
					client:     client,
					playerID:   -1,
					isPlayer:   false,
					gameStarts: make(chan MessageGameStarts),
					newTurn:    make(chan MessageTurn, 100),
					gameEnds:   make(chan MessageGameEnds, 1),
				}

				globalState.Visus = append(globalState.Visus, pvClient)

				log.WithFields(log.Fields{
					"nickname":       client.nickname,
					"remote address": client.Conn.RemoteAddr(),
					"visu count":     len(globalState.Visus),
				}).Info("New visualization accepted")
				client.state = CLIENT_LOGGED

				UnlockGlobalStateMutex(globalState, "New client", "Login manager")

				// Automatically start the game if conditions are met
				autostart(globalState)

				// Visu behavior is handled in dedicated function.
				handlePlayerOrVisu(pvClient, globalState)
			}
		}
	case "game logic":
		if globalState.GameState != GAME_NOT_RUNNING {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: Game has been started")
		} else if len(globalState.GameLogic) >= 1 {
			UnlockGlobalStateMutex(globalState, "New client", "Login manager")
			Kick(client, "LOGIN denied: A game logic is already logged in")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				UnlockGlobalStateMutex(globalState, "New client", "Login manager")
				Kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				glClient := &GameLogicClient{
					client:             client,
					playerAction:       make(chan MessageDoTurnPlayerAction, 1),
					playerDisconnected: make(chan int, 1),
					start:              make(chan int, 1),
				}

				globalState.GameLogic = append(globalState.GameLogic, glClient)

				log.WithFields(log.Fields{
					"nickname":       client.nickname,
					"remote address": client.Conn.RemoteAddr(),
				}).Info("Game logic accepted")

				UnlockGlobalStateMutex(globalState, "New client", "Login manager")

				// Automatically start the game if conditions are met
				autostart(globalState)

				// Game logic behavior is handled in dedicated function
				handleGameLogic(glClient, globalState, gameLogicExit)
			}
		}
	}
}

func Kick(client *Client, reason string) {
	if client.state == CLIENT_KICKED {
		return
	}

	client.state = CLIENT_KICKED
	log.WithFields(log.Fields{
		"remote address": client.Conn.RemoteAddr(),
		"nickname":       client.nickname,
		"reason":         reason,
	}).Warn("Kicking client")

	msg := MessageKick{
		MessageType: "KICK",
		KickReason:  reason,
	}

	content, err := json.Marshal(msg)
	if err == nil {
		_ = sendMessage(client, content)
	}
}

func sendLoginACK(client *Client) error {
	msg := MessageLoginAck{
		MessageType:         "LOGIN_ACK",
		MetaprotocolVersion: Version,
	}

	content, err := json.Marshal(msg)
	if err == nil {
		err = sendMessage(client, content)
	}
	return err
}

func Cleanup() {
	LockGlobalStateMutex(globalGS, "Cleanup", "Main")
	log.Warn("Closing listening socket.")
	globalGS.Listener.Close()

	nonGlClients := append([]*PlayerOrVisuClient(nil), globalGS.Players...)
	nonGlClients = append(nonGlClients, globalGS.SpecialPlayers...)
	nonGlClients = append(nonGlClients, globalGS.Visus...)
	nbClients := len(nonGlClients) + len(globalGS.GameLogic)

	if nbClients > 0 {
		log.Warn("Sending KICK messages to clients")

		kickChan := make(chan int)
		for _, client := range nonGlClients {
			go func(c *Client) {
				c.canTerminate <- "netorcai abort"
				kickChan <- 0
			}(client.client)
		}

		for _, client := range globalGS.GameLogic {
			go func(c *Client) {
				c.canTerminate <- "netorcai abort"
				kickChan <- 0
			}(client.client)
		}

		for i := 0; i < nbClients; i++ {
			<-kickChan
		}
	}

	if globalGS.prompt != nil {
		log.Warn("Cleaning prompt state.")
		globalGS.prompt.TearDown()
	}

	UnlockGlobalStateMutex(globalGS, "Cleanup", "Main")
}
