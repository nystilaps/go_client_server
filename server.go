package main

import (
	"flag"
	"fmt"
	"net"
	"time"
	"strconv"
	"runtime"
	"math/rand"
	"sync/atomic"
	"sync"
	"encoding/binary"
)

const (
	GUESS_NO_MONEY_LEFT = 0
	GUESS_WRONG = 1
	GUESS_RIGHT = 2
	GUESS_BONUS_GAME = 3
	
	SizeOfLuckyBytesChannel = 100
	RenovationPreiod = 10 * time.Second
)

var (
	Jackpot uint64 = 0
	// Channel for lucky bytes of limited size
	LuckyBytesChannel = make(chan uint16, SizeOfLuckyBytesChannel)
	RenovationTimer = time.NewTimer(RenovationPreiod)
	
	// How many times can a user make a guess for free
	UserToNumberOfFreeGames = struct{
		sync.RWMutex
		gamesAvailable map[uint32]uint64
	}{ gamesAvailable: make(map[uint32]uint64) }
)

func populateLuckyBytes() {
	for {
		LuckyBytesChannel <- uint16(rand.Intn(65536))
	}
}

func renovateLuckyBytes() {
	for {
	        select {
		case <-RenovationTimer.C:
			// Remove one lucky pair
			<-LuckyBytesChannel
			// Set up a new timer in 10 seconds
			RenovationTimer.Reset(RenovationPreiod)
			fmt.Println("Renovated luicky pair")
		}
	}
}

func handleConnection(connection net.Conn) {
	// Close connection on function exit
	defer connection.Close()

	// Where is out player from
	client := connection.RemoteAddr().String()
	fmt.Printf("Serving user from %s\n", client)

	// Get lucky bytes for a new connection
	currentLuckyBytes := <-LuckyBytesChannel

	var (
		userId uint32
		fee uint64
		isUserOutOfFreeGames = false
	)

	// Read user id
	if err := binary.Read(connection, binary.LittleEndian, &userId); err != nil {
		fmt.Printf("Stopped serving %s because of error: %s\n", client, err)
		return
	}
	
	// Read prepaid amount
	if err := binary.Read(connection, binary.LittleEndian, &fee); err != nil {
		fmt.Printf("Stopped serving %s because of error: %s\n", client, err)
		return
	}
	
	fmt.Printf("Serving %s as userId=%d for fee=%d\n", client, userId, fee)
	
	// Serve guesses
	for {
		// Read a guess
		var guess uint16
		if err := binary.Read(connection, binary.LittleEndian, &guess); err != nil {
			fmt.Printf("Stopped serving %s because of error: %s\n", client, err)
                        return
		}

		// Reset timer to renovate lucky pair in 10 seconds, if no rouds played
		RenovationTimer.Reset(RenovationPreiod)

		// Check if a guess is allowed
		var guessResult byte
		var isGuessAllowed bool
		if (fee > 0) {
			atomic.AddUint64(&Jackpot, uint64(1))
			fee--
			
			guessResult = GUESS_WRONG
			isGuessAllowed = true
		} else {
			// User has no money left and cannot play
			guessResult = GUESS_NO_MONEY_LEFT
			isGuessAllowed = false
			
			// Unless user can play for free
			UserToNumberOfFreeGames.Lock()
			if UserToNumberOfFreeGames.gamesAvailable[userId] > 0 {
				UserToNumberOfFreeGames.gamesAvailable[userId]--
				guessResult = GUESS_WRONG
				isGuessAllowed = true
			}
			// Clean up zero records
			if UserToNumberOfFreeGames.gamesAvailable[userId] == 0 { 
				delete(UserToNumberOfFreeGames.gamesAvailable, userId)
				isUserOutOfFreeGames = true
			}
			UserToNumberOfFreeGames.Unlock()
		}

		var winAmount uint64 = 0

		// Process a winning guess
		if isGuessAllowed && guess == currentLuckyBytes {
			guessResult = GUESS_RIGHT
			
			// Get new lucky bytes
			currentLuckyBytes = <-LuckyBytesChannel
			// Get amount won in a game
			winAmount = atomic.SwapUint64(&Jackpot, 0)

			fee += winAmount
			
			fmt.Printf("GUESSED by user %d from %s won amount %d balance %d\n", userId, client, winAmount, fee)

			// Process win of zero amount
			if winAmount == 0 {
				guessResult = GUESS_BONUS_GAME
				UserToNumberOfFreeGames.Lock()
				UserToNumberOfFreeGames.gamesAvailable[userId]++
				isUserOutOfFreeGames = false
				UserToNumberOfFreeGames.Unlock()
			}
		}

		// Write type of guess outcome
		connection.SetWriteDeadline(time.Now().Add(1 * time.Second))
		if err := binary.Write(connection, binary.LittleEndian, &guessResult); err != nil {
			fmt.Printf("Stopped serving %s because of error: %s\n", client, err)
			return
		}

		// Write amount won in a game
		if guessResult == GUESS_RIGHT {
		  	connection.SetWriteDeadline(time.Now().Add(1 * time.Second))
			if err := binary.Write(connection, binary.LittleEndian, &winAmount); err != nil {
				fmt.Printf("Stopped serving %s because of error: %s\n", client, err)
				return
			}
		}
		
		// Disconnect user if he has no money and no free games
		if fee == 0 && isUserOutOfFreeGames {
			fmt.Printf("Game over for userId=%d from %s because of low funds.\n", userId, client)
			break
		}
	}
}

func main() {
	var (
		port = flag.Int("port", 8000, "The port to connect to; defaults to 8000.")
		numberOfProcessors = flag.Int("procs", 3, "The number of processors to use; defaults to 3.")
	)
	
	// Process command line
	flag.Parse()
	
	fmt.Println("Server running on port ", *port, " with number of used processors ", *numberOfProcessors)
	
	// Sen number of processors available
	runtime.GOMAXPROCS(*numberOfProcessors)

	// Run renovation and population of lucky bytes goroutimes
        go populateLuckyBytes()
        go renovateLuckyBytes()

	// Listen on given port
	listener, err := net.Listen("tcp4", ":" + strconv.Itoa(*port))
	if err != nil {
		panic(err)
	}
	
	// Close listener on function exit
	defer listener.Close()
	
	// Random seed from time
	rand.Seed(time.Now().Unix())

	// Serve new connections
	for {
		// On new connection
		connection, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		// Run a goroutine for it
		go handleConnection(connection)
	}
}
