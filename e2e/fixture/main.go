package main

import (
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "exit" {
		code, err := strconv.Atoi(os.Args[2])
		if err != nil {
			code = 1
		}
		os.Exit(code)
	}
	if len(os.Args) > 3 && os.Args[1] == "forward" {
		forward(os.Args[2], os.Args[3])
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	<-signals
}

func forward(listen, target string) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		os.Exit(3)
	}
	for {
		inbound, err := listener.Accept()
		if err != nil {
			os.Exit(4)
		}
		go pipe(inbound, target)
	}
}

func pipe(inbound net.Conn, target string) {
	outbound, err := net.Dial("tcp", target)
	if err != nil {
		_ = inbound.Close()
		return
	}
	go func() {
		_, _ = io.Copy(outbound, inbound)
		_ = outbound.Close()
	}()
	_, _ = io.Copy(inbound, outbound)
	_ = inbound.Close()
}
