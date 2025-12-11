package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	address := "127.0.0.1:15985"
	timeout := 2 * time.Second
	fmt.Printf("Dialing %s with timeout %v...\n", address, timeout)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
	} else {
		fmt.Printf("Success!\n")
		conn.Close()
	}
}

