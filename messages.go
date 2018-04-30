package main

import (
	"fmt"
	"regexp"
)

type MessageLogin struct {
	nickname string
	role     string
}

type MessageLoginAck struct {
	MessageType string `json:"message_type"`
}

type MessageGameStarts struct {
	PlayerID       int                    `json:"player_id"`
	NbPlayers      int                    `json:"nb_players"`
	NbTurnsMax     int                    `json:"nb_turns_max"`
	DelayFirstTurn float64                `json:"milliseconds_before_first_turn"`
	Data           map[string]interface{} `json:"data"`
}

type MessageGameEnds struct {
	WinnerPlayerID int                    `json:"winner_player_id"`
	Data           map[string]interface{} `json:"data"`
}

type MessageTurn struct {
	MessageType string                 `json:"message_type"`
	TurnNumber  int                    `json:"turn_number"`
	GameState   map[string]interface{} `json:"game_state"`
}

type MessageTurnAck struct {
	turnNumber int
	actions    map[string]interface{}
}

type MessageKick struct {
	MessageType string `json:"message_type"`
	KickReason  string `json:"kick_reason"`
}

func checkMessageType(data map[string]interface{}, expectedMessageType string) error {
	messageType, err := readString(data, "message_type")
	if err != nil {
		return err
	}

	if messageType != expectedMessageType {
		return fmt.Errorf("Received '%v' message type, "+
			"while %v was expected", messageType, expectedMessageType)
	}

	return nil
}

func readLoginMessage(data map[string]interface{}) (MessageLogin, error) {
	var readMessage MessageLogin

	// Check message type
	err := checkMessageType(data, "LOGIN")
	if err != nil {
		return readMessage, err
	}

	// Read nickname
	readMessage.nickname, err = readString(data, "nickname")
	if err != nil {
		return readMessage, err
	}

	// Check nickname
	r, _ := regexp.Compile(`\A\S{1,10}\z`)
	if !r.MatchString(readMessage.nickname) {
		return readMessage, fmt.Errorf("Invalid nickname")
	}

	// Read role
	readMessage.role, err = readString(data, "role")
	if err != nil {
		return readMessage, err
	}

	// Check role
	switch readMessage.role {
	case "player",
		"visualization",
		"game logic":
		return readMessage, nil
	default:
		return readMessage, fmt.Errorf("Invalid role '%v'",
			readMessage.role)
	}
}

func readTurnACKMessage(data map[string]interface{}, expectedTurnNumber int) (
	MessageTurnAck, error) {
	var readMessage MessageTurnAck

	// Check message type
	err := checkMessageType(data, "TURN_ACK")
	if err != nil {
		return readMessage, err
	}

	// Read turn number
	readMessage.turnNumber, err = readInt(data, "turn_number")
	if err != nil {
		return readMessage, err
	}

	// Check turn number
	if readMessage.turnNumber != expectedTurnNumber {
		return readMessage, fmt.Errorf("Invalid value (turn_number=%v): "+
			"expecting %v", readMessage.turnNumber, expectedTurnNumber)
	}

	// Read actions
	readMessage.actions, err = readObject(data, "actions")
	if err != nil {
		return readMessage, err
	}

	return readMessage, nil
}
