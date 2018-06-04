package test

import (
	//"github.com/stretchr/testify/assert"
	"fmt"
	"regexp"
	"testing"
)

func TestHelloGLOnly(t *testing.T) {
	proc, _, players, visus, gl := runNetorcaiAndAllClients(
		t, []string{"--delay-first-turn=500", "--nb-turns-max=2",
			"--delay-turns=500", "--debug"}, 1000)
	defer killallNetorcaiSIGKILL()

	// Disconnect all players
	for _, player := range players {
		player.Disconnect()
		waitOutputTimeout(regexp.MustCompile(`Remote endpoint closed`),
			proc.outputControl, 1000, false)
	}

	// Disconnect all visus
	for _, visu := range visus {
		visu.Disconnect()
		waitOutputTimeout(regexp.MustCompile(`Remote endpoint closed`),
			proc.outputControl, 1000, false)
	}

	// Run a game client
	go helloGameLogic(t, gl[0], 0, 2, 2,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		regexp.MustCompile(`Game is finished`))

	// Start the game
	proc.inputControl <- "start"

	// Wait for game end
	waitOutputTimeout(regexp.MustCompile(`Game is finished`),
		proc.outputControl, 5000, false)
	waitCompletionTimeout(proc.completion, 1000)
}

func TestHelloGLIdleClients(t *testing.T) {
	proc, _, _, _, gl := runNetorcaiAndAllClients(
		t, []string{"--delay-first-turn=500", "--nb-turns-max=2",
			"--delay-turns=500", "--debug"}, 1000)
	defer killallNetorcaiSIGKILL()

	// Run a game client
	go helloGameLogic(t, gl[0], 4, 2, 2,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		regexp.MustCompile(`Game is finished`))

	// Start the game
	proc.inputControl <- "start"

	// Wait for game end
	waitOutputTimeout(regexp.MustCompile(`Game is finished`),
		proc.outputControl, 5000, false)
	waitCompletionTimeout(proc.completion, 1000)
}

func TestHelloGLActiveVisu(t *testing.T) {
	proc, _, players, visus, gl := runNetorcaiAndAllClients(
		t, []string{"--delay-first-turn=500", "--nb-turns-max=3",
			"--delay-turns=500", "--debug", "--json-logs"}, 1000)
	defer killallNetorcaiSIGKILL()

	// Run a game client
	go helloGameLogic(t, gl[0], 0, 3, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		regexp.MustCompile(`Game is finished`))

	// Disconnect players
	for _, player := range players {
		player.Disconnect()
		waitOutputTimeout(regexp.MustCompile(`Remote endpoint closed`),
			proc.outputControl, 1000, false)
	}

	// Run visu clients
	for _, visu := range visus {
		go helloClient(t, visu, 0, 3, 3, 500, 500, false, true, true,
			DefaultHelloClientTurnAck, regexp.MustCompile(`Game is finished`))
	}

	// Start the game
	proc.inputControl <- "start"

	// Wait for game end
	waitOutputTimeout(regexp.MustCompile(`Game is finished`),
		proc.outputControl, 5000, false)
	waitCompletionTimeout(proc.completion, 1000)
}

func TestHelloGLActivePlayer(t *testing.T) {
	proc, _, players, visus, gl := runNetorcaiAndAllClients(
		t, []string{"--delay-first-turn=500", "--nb-turns-max=3",
			"--delay-turns=500", "--debug", "--json-logs"}, 1000)
	defer killallNetorcaiSIGKILL()

	// Run a game client
	go helloGameLogic(t, gl[0], 1, 3, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		regexp.MustCompile(`Game is finished`))

	// Run an active player
	go helloClient(t, players[0], 1, 3, 3, 500, 500, true, true, true,
		DefaultHelloClientTurnAck, regexp.MustCompile(`Game is finished`))

	// Disconnect other players
	for _, player := range players[1:] {
		player.Disconnect()
		waitOutputTimeout(regexp.MustCompile(`Remote endpoint closed`),
			proc.outputControl, 1000, false)
	}

	// Disconnect visus
	for _, visu := range visus {
		visu.Disconnect()
		waitOutputTimeout(regexp.MustCompile(`Remote endpoint closed`),
			proc.outputControl, 1000, false)
	}

	// Start the game
	proc.inputControl <- "start"

	// Wait for game end
	waitOutputTimeout(regexp.MustCompile(`Game is finished`),
		proc.outputControl, 5000, false)
	waitCompletionTimeout(proc.completion, 1000)
}

func subtestHelloGlActiveClients(t *testing.T,
	nbPlayers, nbVisus int,
	nbTurnsNetorcai, nbTurnsGL, nbTurnsPlayer, nbTurnsVisu int,
	doInitAckFunc GLDoInitAckFunc, doTurnAckFunc GLDoTurnAckFunc,
	playerTurnAckFunc, visuTurnAckFunc ClientTurnAckFunc,
	glKickReasonMatcher, playerKickReasonMatcher,
	visuKickReasonMatcher *regexp.Regexp) {
	proc, _, players, visus, gl := runNetorcaiAndClients(
		t, []string{"--delay-first-turn=500", "--nb-turns-max=3",
			"--delay-turns=500", "--debug", "--json-logs"}, 1000, nbPlayers,
		nbVisus)
	defer killallNetorcaiSIGKILL()

	// Run a game client
	go helloGameLogic(t, gl[0], nbPlayers, nbTurnsNetorcai, nbTurnsGL,
		doInitAckFunc, doTurnAckFunc, glKickReasonMatcher)

	// Run player clients
	for _, player := range players {
		go helloClient(t, player, nbPlayers, 3, nbTurnsPlayer, 500, 500, true,
			nbTurnsPlayer == nbTurnsNetorcai, nbTurnsGL > 0,
			playerTurnAckFunc, playerKickReasonMatcher)
	}

	// Run visu clients
	for _, visu := range visus {
		go helloClient(t, visu, nbPlayers, 3, nbTurnsVisu, 500, 500, false,
			nbTurnsVisu == nbTurnsNetorcai, nbTurnsGL > 0,
			visuTurnAckFunc, visuKickReasonMatcher)
	}

	// Start the game
	proc.inputControl <- "start"

	// Wait for game end
	waitOutputTimeout(regexp.MustCompile(`Game is finished`),
		proc.outputControl, 5000, false)
	waitCompletionTimeout(proc.completion, 1000)
}

func TestHelloGLActiveClients(t *testing.T) {
	subtestHelloGlActiveClients(t, 4, 1,
		3, 3, 3, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		DefaultHelloClientTurnAck, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Game is finished`))
}

// Invalid DO_INIT_ACK
func doInitAckNoMsgType(nbPlayers, nbTurns int) string {
	return `{"initial_game_state":{"all_clients":{}}}`
}

func doInitAckNoInitialGameState(nbPlayers, nbTurns int) string {
	return `{"message_type": "DO_INIT_ACK"}`
}

func doInitAckBadMsgType(nbPlayers, nbTurns int) string {
	return `{"message_type": "DO_INIT_ACKz",
		"initial_game_state":{"all_clients":{}}}`
}

func doInitAckBadInitialGameStateNotObject(nbPlayers, nbTurns int) string {
	return `{"message_type":"DO_INIT_ACK", "initial_game_state":0}`
}

func doInitAckBadInitialGameStateNoAllClients(nbPlayers, nbTurns int) string {
	return `{"message_type":"DO_INIT_ACK", "initial_game_state":{}}`
}

func TestInvalidDoInitAckNoMsgType(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 0, 1, 1,
		doInitAckNoMsgType, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Invalid DO_INIT_ACK message. `+
			`Field 'message_type' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoInitAckNoInitialGameState(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 0, 1, 1,
		doInitAckNoInitialGameState, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Invalid DO_INIT_ACK message. `+
			`Field 'initial_game_state' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoInitAckBadMsgType(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 0, 1, 1,
		doInitAckBadMsgType, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`DO_INIT_ACK was expected`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoInitAckBadInitialGameStateNotObject(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 0, 1, 1,
		doInitAckBadInitialGameStateNotObject, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Non-object value for field 'initial_game_state'`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoInitAckBadInitialGameStateNoAllClients(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 0, 1, 1,
		doInitAckBadInitialGameStateNoAllClients, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Field 'all_clients' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

// Invalid DO_TURN_ACK
func doTurnAckNoMsgType(turn int, actions []interface{}) string {
	return `{"winner_player_id":-1, "game_state":{"all_clients":{}}}`
}

func doTurnAckNoWinner(turn int, actions []interface{}) string {
	return `{"message_type":"DO_TURN_ACK", "game_state":{"all_clients":{}}}`
}

func doTurnAckNoGameState(turn int, actions []interface{}) string {
	return `{"message_type":"DO_TURN_ACK", "winner_player_id":-1}`
}

func doTurnAckNoAllClients(turn int, actions []interface{}) string {
	return `{"message_type":"DO_TURN_ACK", "winner_player_id":-1, ` +
		`"game_state":{}}`
}

func doTurnAckBadWinner(turn int, actions []interface{}) string {
	return `{"message_type":"DO_TURN_ACK", "winner_player_id": 42,` +
		`"game_state":{"all_clients":{}}}`
}

func TestInvalidDoTurnAckNoMsgType(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 1, 0, 0,
		DefaultHelloGLDoInitAck, doTurnAckNoMsgType,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Field 'message_type' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoTurnAckNoWinner(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 1, 0, 0,
		DefaultHelloGLDoInitAck, doTurnAckNoWinner,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Field 'winner_player_id' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoTurnAckNoGameState(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 1, 0, 0,
		DefaultHelloGLDoInitAck, doTurnAckNoGameState,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Field 'game_state' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoTurnAckNoAllClients(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 1, 0, 0,
		DefaultHelloGLDoInitAck, doTurnAckNoAllClients,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Field 'all_clients' is missing`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

func TestInvalidDoTurnAckBadWinner(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 1, 0, 0,
		DefaultHelloGLDoInitAck, doTurnAckBadWinner,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Invalid winner_player_id: Not in \[-1, 1\[`),
		regexp.MustCompile(`netorcai abort`),
		regexp.MustCompile(`netorcai abort`))
}

// Invalid TURN_ACK
func turnAckNoMsgType(turn int) string {
	return fmt.Sprintf(`{"turn_number": %v, "actions": []}`, turn)
}

func turnAckNoTurnNumber(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACK", "actions": []}`)
}

func turnAckNoActions(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACK",
		"turn_number": %v}`, turn)
}

func turnAckBadMsgType(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACKz",
		"turn_number": %v, "actions": []}`, turn)
}

func turnAckBadTurnNumberValue(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACK",
		"turn_number": %v, "actions": []}`, turn+1)
}

func turnAckBadTurnNumberNotInt(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACK",
		"turn_number": "nope", "actions": []}`)
}

func turnAckBadActions(turn int) string {
	return fmt.Sprintf(`{"message_type": "TURN_ACK",
		"turn_number": %v, "actions": {}}`, turn)
}

func TestInvalidTurnAckNoMsgType(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckNoMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Field 'message_type' is missing`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckNoTurnNumber(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckNoTurnNumber, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Field 'turn_number' is missing`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckNoActions(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckNoActions, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Field 'actions' is missing`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckBadMsgType(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckBadMsgType, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`TURN_ACK was expected`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckBadTurnNumberValue(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckBadTurnNumberValue, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Invalid value \(turn_number=1\)`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckBadTurnNumberNotInt(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckBadTurnNumberNotInt, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Non-integral value for field 'turn_number'`),
		regexp.MustCompile(`Game is finished`))
}

func TestInvalidTurnAckBadActions(t *testing.T) {
	subtestHelloGlActiveClients(t, 1, 1,
		3, 3, 2, 3,
		DefaultHelloGLDoInitAck, DefaultHelloGlDoTurnAck,
		turnAckBadActions, DefaultHelloClientTurnAck,
		regexp.MustCompile(`Game is finished`),
		regexp.MustCompile(`Non-array value for field 'actions'`),
		regexp.MustCompile(`Game is finished`))
}