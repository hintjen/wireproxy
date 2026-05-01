package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	ColorGreen  = "\033[0;32m"
	ColorBlue   = "\033[0;34m"
	ColorCyan   = "\033[0;36m"
	ColorYellow = "\033[1;33m"
	ColorRed    = "\033[0;31m"
	ColorNC     = "\033[0m"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  Server: go run wireworm_chat.go server <port>")
		fmt.Println("  Client: go run wireworm_chat.go client <addr:port>")
		return
	}

	mode := os.Args[1]
	target := os.Args[2]

	var conn net.Conn
	var err error

	const greeting = "WORM\n"

	if mode == "server" {
		fmt.Printf(ColorCyan+"Chat Server listening on 127.0.0.1:%s..."+ColorNC+"\n", target)
		ln, err := net.Listen("tcp", "127.0.0.1:"+target)
		if err != nil {
			log.Fatal(err)
		}
		conn, err = ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// Send a readiness greeting so the client can confirm the
		// upstream chat server is actually here (and not just a
		// half-open wireproxy tunnel whose far end hasn't dialed
		// the chat server yet).
		if _, err := conn.Write([]byte(greeting)); err != nil {
			log.Fatalf("failed to send greeting: %v", err)
		}
		fmt.Println(ColorGreen + "Peer connected! Start typing (type '/ping' for RTT, Ctrl+C to quit)." + ColorNC)
	} else {
		fmt.Printf(ColorCyan+"Connecting to Chat Server at %s..."+ColorNC+"\n", target)
		// Retry connection until the host's chat server actually answers.
		// The tunnel itself comes up before the chat server has finished
		// starting, so a single Dial+EOF is the symptom; we re-try until
		// we receive the greeting.
		deadline := time.Now().Add(60 * time.Second)
		for {
			conn, err = net.Dial("tcp", target)
			if err != nil {
				if time.Now().After(deadline) {
					log.Fatalf("dial failed: %v", err)
				}
				time.Sleep(1 * time.Second)
				continue
			}
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, len(greeting))
			n, rerr := io.ReadFull(conn, buf)
			conn.SetReadDeadline(time.Time{})
			if rerr == nil && n == len(greeting) && string(buf) == greeting {
				break
			}
			_ = conn.Close()
			if time.Now().After(deadline) {
				log.Fatal("timeout waiting for chat server greeting")
			}
			fmt.Println(ColorYellow + "Server not ready yet, retrying..." + ColorNC)
			time.Sleep(1 * time.Second)
		}
		fmt.Println(ColorGreen + "Connected to peer! Start typing (type '/ping' for RTT, Ctrl+C to quit)." + ColorNC)
	}

	defer conn.Close()

	// Receiver loop
	go func() {
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Println("\n" + ColorYellow + "Peer disconnected." + ColorNC)
				} else {
					fmt.Printf("\n"+ColorRed+"Connection error: %v"+ColorNC+"\n", err)
				}
				os.Exit(0)
			}

			line = strings.TrimSpace(line)

			// Internal protocol
			if strings.HasPrefix(line, "PONG:") {
				var ts int64
				fmt.Sscanf(line, "PONG:%d", &ts)
				sentTime := time.Unix(0, ts)
				rtt := time.Since(sentTime)
				fmt.Printf("\r"+ColorGreen+"[LATENCY]"+ColorNC+" Round-trip time: %v\n", rtt)
				fmt.Print("You: ")
				continue
			}

			if strings.HasPrefix(line, "PING:") {
				_, _ = fmt.Fprintf(conn, "PONG:%s\n", line[5:])
				continue
			}

			// Clean print for user
			fmt.Printf("\r%s\n", line)
			fmt.Print("You: ")
		}
	}()

	fmt.Print("You: ")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()
		trimmed := strings.TrimSpace(text)

		if trimmed == "" {
			fmt.Print("You: ")
			continue
		}

		if trimmed == "/ping" {
			now := time.Now().UnixNano()
			_, _ = fmt.Fprintf(conn, "PING:%d\n", now)
			fmt.Printf(ColorBlue + "[INFO]" + ColorNC + " Pinging peer...\n")
		} else {
			_, err := fmt.Fprintf(conn, "Peer: %s\n", text)
			if err != nil {
				break
			}
		}
		fmt.Print("You: ")
	}
}
