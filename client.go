package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
	"math/rand"
	"encoding/binary"
)

const (
	GUESS_NO_MONEY_LEFT = 0
	GUESS_WRONG = 1
	GUESS_RIGHT = 2
	GUESS_BONUS_GAME = 3
)

func main() {
	var (
		host = flag.String("host", "localhost", "The hostname or IP to connect to; defaults to \"localhost\".")
		port = flag.Int("port", 8000, "The port to connect to; defaults to 8000.")
	)
  
	// Get command line
	flag.Parse()

	// Destination for connection
	destination := *host + ":" + strconv.Itoa(*port)
	fmt.Printf("Connecting to %s...\n", destination)

	// Connect to remote host:port
	connection, err := net.Dial("tcp", destination)
	if err != nil {
		fmt.Println("Problem connecting: ", err)
		os.Exit(1)
	}

	// Set random seed
	rand.Seed(time.Now().Unix())
	
	// Do work
	processConnection(connection)
}

func processConnection(connection net.Conn) {
	// Close connection on exit
	defer connection.Close()

	var (
		fee uint64 = 256 * 256;
		userId uint32 = uint32(rand.Intn(65536))
		freeGames uint64 = 0
	)
	
	// Write user id
	connection.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if err := binary.Write(connection, binary.LittleEndian, userId); err != nil {
		fmt.Println("Error writing to stream: ", err)
		return
	}

	// Write prepaid amount 
	connection.SetWriteDeadline(time.Now().Add(1 * time.Second))
	if err := binary.Write(connection, binary.LittleEndian, fee); err != nil {
		fmt.Println("Error writing to stream: ", err)
		return
	}

	fmt.Printf("Playing as userId=%d for fee=%d\n", userId, fee)

	// Guess all two bytes values one by one
	for guess := uint16(0); ; guess++ {
		if fee + freeGames > 0 {
		        if fee > 0 {
				fee--
			} else {
				freeGames--
			}
			// Send our guess
			connection.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err:= binary.Write(connection, binary.LittleEndian, guess); err != nil {
				fmt.Println("Error writing to stream: ", err)
				break
			}
		} else {
			fmt.Println("I quit. I lost my money, not dignity.")
			return
		}
	  
		// Read game outcome
		connection.SetReadDeadline(time.Now().Add(2 * time.Second))
		var value byte
		if err := binary.Read(connection, binary.LittleEndian, &value); err != nil {
			fmt.Println("Reading reply error: ", err)
			return
		}

		if value == GUESS_RIGHT {
			// Read amount won in a game
			connection.SetReadDeadline(time.Now().Add(2 * time.Second))
			var winAmount uint64 = 0
			if err := binary.Read(connection, binary.LittleEndian, &winAmount); err != nil {
				fmt.Println("Error reading from stream: ", err)
			}
			fee += winAmount
			fmt.Println("GUESSED, won amount ", winAmount, " free games ", freeGames, " balance ", fee)
			
		} else if value == GUESS_BONUS_GAME {
			fmt.Println("GUESSED but only got a free spin", " free games ", freeGames, " balance ", fee)
			freeGames++
		} else if value == GUESS_NO_MONEY_LEFT {
			// Should never get here
			panic("Hey, Someone stole my money!")
		}
	}
}
