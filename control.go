package main

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"net"
	"sync"
	"time"
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
	CLIENT_FINISHED = iota
	CLIENT_KICKED   = iota
)

type PlayerOrVisuClient struct {
	client     *Client
	playerID   int // TODO: generate them when the game is started
	isPlayer   bool
	gameStarts chan MessageGameStarts
	newTurn    chan MessageTurn
	gameEnds   chan MessageGameEnds
}

type GameLogicClient struct {
	client *Client
	// Messages to aggregate from player clients
	playerAction chan MessageDoTurnPlayerAction
	// Control messages
	start chan int
}

type GlobalState struct {
	mutex sync.Mutex

	listener net.Listener

	gameState int

	gameLogic []GameLogicClient
	players   []PlayerOrVisuClient
	visus     []PlayerOrVisuClient

	nbPlayersMax                int
	nbVisusMax                  int
	nbTurnsMax                  int
	millisecondsBeforeFirstTurn float64
	millisecondsBetweenTurns    float64
}

func handleClient(client *Client, globalState *GlobalState,
	gameLogicExit chan int) {
	log.WithFields(log.Fields{
		"remote address": client.conn.RemoteAddr(),
	}).Debug("New connection")

	defer client.conn.Close()

	go readClientMessages(client)

	msg := <-client.incomingMessages
	if msg.err != nil {
		log.WithFields(log.Fields{
			"err":            msg.err,
			"remote address": client.conn.RemoteAddr(),
		}).Debug("Cannot receive client first message")
		kick(client, fmt.Sprintf("Invalid first message: %v", msg.err.Error()))
		return
	}

	loginMessage, err := readLoginMessage(msg.content)
	if err != nil {
		log.WithFields(log.Fields{
			"err":            err,
			"remote address": client.conn.RemoteAddr(),
		}).Debug("Cannot read LOGIN message")
		kick(client, fmt.Sprintf("Invalid first message: %v", err.Error()))
		return
	}
	client.nickname = loginMessage.nickname

	globalState.mutex.Lock()
	switch loginMessage.role {
	case "player":
		if globalState.gameState != GAME_NOT_RUNNING {
			globalState.mutex.Unlock()
			kick(client, "LOGIN denied: Game has been started")
		} else if len(globalState.players) >= globalState.nbPlayersMax {
			globalState.mutex.Unlock()
			kick(client, "LOGIN denied: Maximum number of players reached")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				globalState.mutex.Unlock()
				kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				pvClient := PlayerOrVisuClient{
					client:   client,
					playerID: -1,
					isPlayer: true,
					newTurn:  make(chan MessageTurn),
				}

				globalState.players = append(globalState.players, pvClient)

				log.WithFields(log.Fields{
					"nickname":       client.nickname,
					"remote address": client.conn.RemoteAddr(),
					"player count":   len(globalState.players),
				}).Info("New player accepted")

				globalState.mutex.Unlock()

				// Player behavior is handled in dedicated function.
				handlePlayerOrVisu(&pvClient, globalState)
			}
		}
	case "visualization":
		if len(globalState.visus) >= globalState.nbVisusMax {
			globalState.mutex.Unlock()
			kick(client, "LOGIN denied: Maximum number of visus reached")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				globalState.mutex.Unlock()
				kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				pvClient := PlayerOrVisuClient{
					client:   client,
					playerID: -1,
					isPlayer: false,
					newTurn:  make(chan MessageTurn),
				}

				globalState.visus = append(globalState.visus, pvClient)

				log.WithFields(log.Fields{
					"nickname":       client.nickname,
					"remote address": client.conn.RemoteAddr(),
					"visu count":     len(globalState.visus),
				}).Info("New visualization accepted")

				globalState.mutex.Unlock()

				// Visu behavior is handled in dedicated function.
				handlePlayerOrVisu(&pvClient, globalState)
			}
		}
	case "game logic":
		if globalState.gameState != GAME_NOT_RUNNING {
			globalState.mutex.Unlock()
			kick(client, "LOGIN denied: Game has been started")
		} else if len(globalState.gameLogic) >= 1 {
			globalState.mutex.Unlock()
			kick(client, "LOGIN denied: A game logic is already logged in")
		} else {
			err = sendLoginACK(client)
			if err != nil {
				globalState.mutex.Unlock()
				kick(client, "LOGIN denied: Could not send LOGIN_ACK")
			} else {
				glClient := GameLogicClient{
					client: client,
				}

				globalState.gameLogic = append(globalState.gameLogic, glClient)

				log.WithFields(log.Fields{
					"nickname":       client.nickname,
					"remote address": client.conn.RemoteAddr(),
				}).Info("Game logic accepted")

				globalState.mutex.Unlock()

				handleGameLogic(glClient, globalState, gameLogicExit)
			}
		}
	default:
		globalState.mutex.Unlock()
		kick(client, fmt.Sprintf("LOGIN denied: Unknown role '%v'",
			loginMessage.role))
	}
}

func handlePlayerOrVisu(pvClient *PlayerOrVisuClient,
	globalState *GlobalState) {
	turnBuffer := make([]MessageTurn, 1)
	lastTurnNumberSent := -1

	for {
		select {
		case turn := <-pvClient.newTurn:
			// A new turn has been received.
			if pvClient.client.state == CLIENT_READY {
				// The client is ready, the message can be sent right now.
				lastTurnNumberSent = turn.TurnNumber
				err := sendTurn(pvClient.client, turn)
				if err != nil {
					kickLoggedPlayerOrVisu(pvClient, globalState,
						fmt.Sprintf("Cannot send TURN. %v", err.Error()))
					return
				}
				pvClient.client.state = CLIENT_THINKING
			} else if pvClient.client.state == CLIENT_THINKING {
				// The client is still computing something (its decisions for
				// a player, or just updating its display for a visualization).
				// The turn message therefore buffered.
				if len(turnBuffer) > 0 {
					// Update the turn buffer with the new message.
					turnBuffer[0] = turn
				} else {
					// Put the new message into the turn buffer.
					turnBuffer = append(turnBuffer, turn)
				}
			}
		case msg := <-pvClient.client.incomingMessages:
			// A new message has been received from the player socket.
			if msg.err != nil {
				kickLoggedPlayerOrVisu(pvClient, globalState,
					fmt.Sprintf("Cannot read TURN_ACK. %v", msg.err.Error()))
				return
			}
			turnAckMsg, err := readTurnAckMessage(msg.content,
				lastTurnNumberSent)
			if err != nil {
				kickLoggedPlayerOrVisu(pvClient, globalState,
					fmt.Sprintf("Invalid TURN_ACK received. %v",
						err.Error()))
				return
			}

			// Check client state
			if pvClient.client.state != CLIENT_THINKING {
				kickLoggedPlayerOrVisu(pvClient, globalState,
					"Received a TURN_ACK but the client state is not THINKING")
				return
			}

			// Check turnNumber value
			if turnAckMsg.turnNumber != lastTurnNumberSent {
				kickLoggedPlayerOrVisu(pvClient, globalState,
					fmt.Sprintf("Invalid TURN_ACK received: "+
						"Expected turn_number=%v, got %v", lastTurnNumberSent,
						turnAckMsg.turnNumber))
			}

			if pvClient.isPlayer {
				// Forward the player actions to the game logic
				globalState.mutex.Lock()
				if len(globalState.gameLogic) == 1 {
					globalState.gameLogic[0].playerAction <- MessageDoTurnPlayerAction{
						PlayerID:   pvClient.playerID,
						TurnNumber: turnAckMsg.turnNumber,
						Actions:    turnAckMsg.actions,
					}
				}
				globalState.mutex.Unlock()
			}

			// If a TURN is buffered, send it right now.
			if len(turnBuffer) > 0 {
				err := sendTurn(pvClient.client, turnBuffer[0])
				if err != nil {
					kickLoggedPlayerOrVisu(pvClient, globalState,
						fmt.Sprintf("Cannot send TURN. %v", err.Error()))
					return
				}

				// Empty turn buffer
				turnBuffer = turnBuffer[0:]
				pvClient.client.state = CLIENT_THINKING
			} else {
				pvClient.client.state = CLIENT_READY
			}
		}
	}
}

func handleGameLogic(glClient GameLogicClient, globalState *GlobalState,
	onexit chan int) {
	// Wait for the game to start
	select {
	case <-glClient.start:
	case msg := <-glClient.client.incomingMessages:
		globalState.mutex.Lock()
		if msg.err == nil {
			kick(glClient.client, "Received a game logic message but "+
				"the game has not started")
		} else {
			kick(glClient.client, fmt.Sprintf("Game logic error. %v",
				msg.err.Error()))
		}
		globalState.gameLogic = globalState.gameLogic[:0]
		globalState.mutex.Unlock()
		return
	}

	// Generate randomized player identifiers
	globalState.mutex.Lock()
	playerIDs := rand.Perm(len(globalState.players))
	for playerIndex, player := range globalState.players {
		player.playerID = playerIDs[playerIndex]
	}

	// Send DO_FIRST_TURN
	err := sendDoInit(glClient, len(globalState.players),
		globalState.nbTurnsMax)
	globalState.mutex.Unlock()

	if err != nil {
		kick(glClient.client, fmt.Sprintf("Cannot send DO_INIT. %v",
			err.Error()))
		onexit <- 1
		return
	}

	// Wait for first turn (DO_INIT_ACK)
	msg := <-glClient.client.incomingMessages
	if msg.err != nil {
		kick(glClient.client,
			fmt.Sprintf("Cannot read DO_INIT_ACK. %v", msg.err.Error()))
		onexit <- 1
		return
	}

	doTurnAckMsg, err := readDoInitAckMessage(msg.content)
	if err != nil {
		kick(glClient.client,
			fmt.Sprintf("Invalid DO_INIT_ACK message. %v", msg.err.Error()))
		onexit <- 1
		return
	}

	// Send GAME_STARTS to all clients
	globalState.mutex.Lock()
	initialNbPlayers := len(globalState.players)
	for _, player := range globalState.players {
		player.gameStarts <- MessageGameStarts{
			PlayerID:         player.playerID,
			NbPlayers:        initialNbPlayers,
			NbTurnsMax:       globalState.nbTurnsMax,
			DelayFirstTurn:   globalState.millisecondsBeforeFirstTurn,
			InitialGameState: doTurnAckMsg.InitialGameState,
		}
	}
	globalState.mutex.Unlock()

	// Wait before really starting the game
	time.Sleep(time.Duration(globalState.millisecondsBeforeFirstTurn) *
		time.Millisecond)

	// Order the game logic to compute a TURN (without any action)
	turnNumber := 0
	playerActions := make([]MessageDoTurnPlayerAction, 0)
	sendDoTurn(glClient, playerActions)

	for {
		select {
		case action := <-glClient.playerAction:
			// A client sent its actions.
			// Replace the current message from this player if it exists,
			// and place it at the end of the array.
			// This may happen if the client was late in a previous turn but
			// catched up in current turn by sending two TURN_ACK.
			actionFound := false
			for actionIndex, act := range playerActions {
				if act.PlayerID == action.PlayerID {
					playerActions[len(playerActions)-1], playerActions[actionIndex] = playerActions[actionIndex], playerActions[len(playerActions)-1]
					playerActions[len(playerActions)-1] = action
					break
				}
			}

			if !actionFound {
				// Append the action into the actions array
				playerActions = append(playerActions, action)
			}

		case msg := <-glClient.client.incomingMessages:
			// New message received from the game logic
			if msg.err != nil {
				kick(glClient.client,
					fmt.Sprintf("Cannot read DO_TURN_ACK. %v",
						msg.err.Error()))
				onexit <- 1
				return
			}

			doTurnAckMsg, err := readDoTurnAckMessage(msg.content,
				initialNbPlayers)
			if err != nil {
				kick(glClient.client,
					fmt.Sprintf("Invalid DO_TURN_ACK message. %v",
						err.Error()))
				onexit <- 1
				return
			}

			// Forward the TURN to the clients
			globalState.mutex.Lock()
			for _, player := range globalState.players {
				player.newTurn <- MessageTurn{
					TurnNumber: turnNumber,
					GameState:  doTurnAckMsg.GameState,
				}
			}
			for _, visu := range globalState.visus {
				visu.newTurn <- MessageTurn{
					TurnNumber: turnNumber,
					GameState:  doTurnAckMsg.GameState,
				}
			}
			globalState.mutex.Unlock()
			turnNumber = turnNumber + 1

			if turnNumber < globalState.nbTurnsMax {
				// Trigger a new DO_TURN in some time
				go func() {
					time.Sleep(time.Duration(globalState.millisecondsBetweenTurns) *
						time.Millisecond)

					// Send current actions
					sendDoTurn(glClient, playerActions)
					// Clear actions array
					playerActions = playerActions[:0]
				}()
			} else {
				// Send GAME_ENDS to all clients
				globalState.mutex.Lock()
				for _, player := range globalState.players {
					player.gameEnds <- MessageGameEnds{
						WinnerPlayerID: doTurnAckMsg.WinnerPlayerID,
						GameState:      doTurnAckMsg.GameState,
					}
				}
				for _, visu := range globalState.visus {
					visu.gameEnds <- MessageGameEnds{
						WinnerPlayerID: doTurnAckMsg.WinnerPlayerID,
						GameState:      doTurnAckMsg.GameState,
					}
				}

				globalState.mutex.Unlock()
			}
		}
	}
}

func kick(client *Client, reason string) {
	client.state = CLIENT_KICKED
	log.WithFields(log.Fields{
		"remote address": client.conn.RemoteAddr(),
		"nickname":       client.nickname,
		"reason":         reason,
	}).Warn("Kicking client")

	msg := MessageKick{
		MessageType: "KICK",
		KickReason:  reason,
	}

	content, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot marshal JSON message")
	} else {
		_ = sendMessage(client, content)
		time.Sleep(500 * time.Millisecond)
	}
}

func kickLoggedPlayerOrVisu(pvClient *PlayerOrVisuClient,
	gs *GlobalState, reason string) {
	// Remove the client from the global state
	gs.mutex.Lock()

	if pvClient.isPlayer {
		// Locate the player in the array
		playerIndex := -1
		for index, value := range gs.players {
			if value.client == pvClient.client {
				playerIndex = index
				break
			}
		}

		if playerIndex == -1 {
			log.Error("Could not remove player: Did not find it")
		} else {
			// Remove the player by placing it at the end of the slice,
			// then reducing the slice length
			gs.players[len(gs.players)-1], gs.players[playerIndex] = gs.players[playerIndex], gs.players[len(gs.players)-1]
			gs.players = gs.players[:len(gs.players)-1]
		}
	} else {
		// Locate the visu in the array
		visuIndex := -1
		for index, value := range gs.visus {
			if value.client == pvClient.client {
				visuIndex = index
				break
			}
		}

		if visuIndex == -1 {
			log.Error("Could not remove visu: Did not find it")
		} else {
			// Remove the visu by placing it at the end of the slice,
			// then reducing the slice length
			gs.visus[len(gs.visus)-1], gs.visus[visuIndex] = gs.visus[visuIndex], gs.visus[len(gs.visus)-1]
			gs.visus = gs.visus[:len(gs.visus)-1]
		}
	}

	gs.mutex.Unlock()

	// Kick the client
	kick(pvClient.client, reason)
}

func sendLoginACK(client *Client) error {
	msg := MessageLoginAck{
		MessageType: "LOGIN_ACK",
	}

	content, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot marshal JSON message")
		return err
	} else {
		err = sendMessage(client, content)
		return err
	}
}

func sendTurn(client *Client, msg MessageTurn) error {
	content, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot marshal JSON message")
		return err
	} else {
		err = sendMessage(client, content)
		return err
	}
}

func sendDoInit(client GameLogicClient, nbPlayers, nbTurnsMax int) error {
	msg := MessageDoInit{
		MessageType: "DO_INIT",
		NbPlayers:   nbPlayers,
		NbTurnsMax:  nbTurnsMax,
	}

	content, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot marshal JSON message")
		return err
	} else {
		err = sendMessage(client.client, content)
		return err
	}
}

func sendDoTurn(client GameLogicClient,
	playerActions []MessageDoTurnPlayerAction) error {
	msg := MessageDoTurn{
		MessageType:   "DO_TURN",
		PlayerActions: playerActions,
	}

	content, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Cannot marshal JSON message")
		return err
	} else {
		err = sendMessage(client.client, content)
		return err
	}
}
