package netorcai

import (
	"bufio"
	"fmt"
	"github.com/mpoquet/go-prompt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	globalGS        *GlobalState
	globalShellExit chan int
	globalGameLogicExit chan int
)

func stringInSlice(searchedValue string, slice []string) bool {
	for _, value := range slice {
		if value == searchedValue {
			return true
		}
	}
	return false
}

func executor(line string) {
	line = strings.TrimSpace(line)
	rStart, _ := regexp.Compile(`\Astart\z`)
	rQuit, _ := regexp.Compile(`\Aquit\z`)
	rPrint, _ := regexp.Compile(`\Aprint\s+(?P<variable>\S+)\z`)
	rSet, _ := regexp.Compile(`\Aset\s+(?P<variable>\S+)(?P<sep>\s|=)(?P<value>\S+)\z`)
	rCall, _ := regexp.Compile(`\Acall\s+(?P<url>\S+)\z`)

	acceptedSetVariables := []string{
		"nb-turns-max",
		"nb-players-max",
		"nb-splayers-max",
		"nb-visus-max",
		"delay-first-turn",
		"delay-turns",
	}

	acceptedPrintVariables := append(acceptedSetVariables, "all")

	if rStart.MatchString(line) {
		LockGlobalStateMutex(globalGS, "got start command", "Prompt")
		if globalGS.GameState == GAME_NOT_RUNNING {
			if len(globalGS.GameLogic) == 1 {
				globalGS.GameState = GAME_RUNNING
				globalGS.GameLogic[0].start <- 1
			} else {
				fmt.Printf("Cannot start: Game logic not connected\n")
			}
		} else {
			fmt.Printf("Game has already been started\n")
		}
		UnlockGlobalStateMutex(globalGS, "got start command", "Prompt")
	} else if rQuit.MatchString(line) {
		globalShellExit <- 0
	} else if rPrint.MatchString(line) {
		m := rPrint.FindStringSubmatch(line)
		names := rPrint.SubexpNames()
		matches := map[string]string{}
		for index, matchedString := range m {
			matches[names[index]] = matchedString
		}

		if stringInSlice(matches["variable"], acceptedPrintVariables) {
			switch matches["variable"] {
			case "nb-turns-max":
				fmt.Printf("%v=%v\n", "nb-turns-max", globalGS.NbTurnsMax)
			case "nb-players-max":
				fmt.Printf("%v=%v\n", "nb-players-max",
					globalGS.NbPlayersMax)
			case "nb-splayers-max":
				fmt.Printf("%v=%v\n", "nb-splayers-max",
					globalGS.NbSpecialPlayersMax)
			case "nb-visus-max":
				fmt.Printf("%v=%v\n", "nb-visus-max", globalGS.NbVisusMax)
			case "delay-first-turn":
				fmt.Printf("%v=%v\n", "delay-first-turn",
					globalGS.MillisecondsBeforeFirstTurn)
			case "delay-turns":
				fmt.Printf("%v=%v\n", "delay-turns",
					globalGS.MillisecondsBetweenTurns)
			case "all":
				fmt.Printf("%v=%v\n", "nb-turns-max", globalGS.NbTurnsMax)
				fmt.Printf("%v=%v\n", "nb-players-max",
					globalGS.NbPlayersMax)
				fmt.Printf("%v=%v\n", "nb-splayers-max",
					globalGS.NbSpecialPlayersMax)
				fmt.Printf("%v=%v\n", "nb-visus-max", globalGS.NbVisusMax)
				fmt.Printf("%v=%v\n", "delay-first-turn",
					globalGS.MillisecondsBeforeFirstTurn)
				fmt.Printf("%v=%v\n", "delay-turns",
					globalGS.MillisecondsBetweenTurns)
			}
		} else {
			fmt.Printf("Bad VARIABLE=%v. Accepted values: %v\n",
				matches["variable"],
				strings.Join(acceptedPrintVariables, " "))
		}
	} else if rSet.MatchString(line) {
		m := rSet.FindStringSubmatch(line)
		names := rSet.SubexpNames()
		matches := map[string]string{}
		for index, matchedString := range m {
			matches[names[index]] = matchedString
		}

		if stringInSlice(matches["variable"], acceptedSetVariables) {
			// Read value
			intValue, errInt := strconv.ParseInt(matches["value"], 0, 64)
			floatValue, errFloat := strconv.ParseFloat(matches["value"], 64)

			switch matches["variable"] {
			case "nb-turns-max":
				if errInt != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errInt.Error())
				} else {
					if intValue >= 1 && intValue <= 65535 {
						globalGS.NbTurnsMax = int(intValue)
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [1,65535]\n",
							intValue)
					}
				}
			case "nb-players-max":
				if errInt != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errInt.Error())
				} else {
					if intValue >= 1 && intValue <= 1024 {
						globalGS.NbPlayersMax = int(intValue)
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [1,1024]\n",
							intValue)
					}
				}
			case "nb-splayers-max":
				if errInt != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errInt.Error())
				} else {
					if intValue >= 0 && intValue <= 1024 {
						globalGS.NbSpecialPlayersMax = int(intValue)
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [0,1024]\n",
							intValue)
					}
				}
			case "nb-visus-max":
				if errInt != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errInt.Error())
				} else {
					if intValue >= 0 && intValue <= 1024 {
						globalGS.NbVisusMax = int(intValue)
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [0,1024]\n",
							intValue)
					}
				}
			case "delay-first-turn":
				if errFloat != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errFloat.Error())
				} else {
					if floatValue >= 50 && floatValue <= 10000 {
						globalGS.MillisecondsBeforeFirstTurn = floatValue
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [50,10000]\n",
							floatValue)
					}
				}
			case "delay-turns":
				if errFloat != nil {
					fmt.Printf("Bad VALUE=%v. %v\n",
						matches["value"], errFloat.Error())
				} else {
					if floatValue >= 50 && floatValue <= 10000 {
						globalGS.MillisecondsBetweenTurns = floatValue
					} else {
						fmt.Printf("Bad VALUE=%v: Not in [50,10000]\n",
							floatValue)
					}
				}
			}
		} else {
			fmt.Printf("Bad VARIABLE=%v. Accepted values: %v\n",
				matches["variable"],
				strings.Join(acceptedSetVariables, " "))
		}
	} else if rCall.MatchString(line){
		m := rCall.FindStringSubmatch(line)
		names := rCall.SubexpNames()
		matches := map[string]string{}
		for index, matchedString := range m {
			matches[names[index]] = matchedString
		}
		url := matches["url"]
		go callAgent(url, globalGS, globalGameLogicExit)
	} else {
		if strings.HasPrefix(line, "start") {
			fmt.Println("expected syntax: start")
		} else if strings.HasPrefix(line, "quit") {
			fmt.Println("expected syntax: quit")
		} else if strings.HasPrefix(line, "print") {
			fmt.Println("expected syntax: print VARIABLE")
		} else if strings.HasPrefix(line, "set") {
			fmt.Println("expected syntax: set VARIABLE=VALUE\n" +
				"   (alt syntax): set VARIABLE VALUE")
		}  else if strings.HasPrefix(line, "call") {
			fmt.Println("expected syntax: set NICK@URL\n")
		}
	}
}

func completer(d prompt.Document) []prompt.Suggest {
	commandsSugestions := []prompt.Suggest{
		{Text: "start", Description: "Start the game"},
		{Text: "print VARIABLE", Description: "Print value of variable"},
		{Text: "set VARIABLE=VALUE", Description: "Set value of variable"},
		{Text: "call SERVER:PORT", Description: "Call an agent to join the game"},
		{Text: "quit", Description: "Quit netorcai"},
	}

	setSuggestions := []prompt.Suggest{
		{Text: "nb-turns-max", Description: "Maximum number of turns"},
		{Text: "nb-players-max", Description: "Maximum number of players"},
		{Text: "nb-splayers-max", Description: "Maximum number of special players"},
		{Text: "nb-visus-max", Description: "Maximum number of visualizations"},
		{Text: "delay-first-turn", Description: "Time (ms) before 1st turn"},
		{Text: "delay-turns", Description: "Time (ms) between turns"},
	}

	printSuggestions := append(setSuggestions, prompt.Suggest{Text: "all",
		Description: "Print the value of all variables"})

	t := d.TextBeforeCursor()

	if strings.Count(t, " ") == 0 {
		return prompt.FilterHasPrefix(commandsSugestions, t, true)
	} else if strings.HasPrefix(t, "print") {
		return prompt.FilterHasPrefix(printSuggestions,
			strings.TrimPrefix(t, "print "), true)
	} else if strings.HasPrefix(t, "set") {
		return prompt.FilterHasPrefix(setSuggestions,
			strings.TrimPrefix(t, "set "), true)
	} else {
		return []prompt.Suggest{}
	}
}

func RunPrompt(gs *GlobalState, onexit chan int, gameLogicExit chan int, interactive bool) {
	globalGS = gs
	globalShellExit = onexit
	globalGameLogicExit = gameLogicExit

	if interactive {
		interactivePrompt(onexit)
	} else {
		nonInteractivePrompt(onexit)
	}

}

func interactivePrompt(onexit chan int) {
	LockGlobalStateMutex(globalGS, "Creating prompt", "Prompt")
	globalGS.prompt = prompt.New(
		executor,
		completer,
		prompt.OptionPrefix(">>> "),
		prompt.OptionTitle(""),
	)
	UnlockGlobalStateMutex(globalGS, "Creating prompt", "Prompt")

	globalGS.prompt.Run()
	onexit <- 1
}

func nonInteractivePrompt(onexit chan int) {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, _ := reader.ReadString('\n')
		executor(line)
	}
}
