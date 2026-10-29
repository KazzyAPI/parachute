package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "exit" {
		code, err := strconv.Atoi(os.Args[2])
		if err != nil {
			code = 1
		}
		os.Exit(code)
	}
	if path := os.Getenv("WRITE"); path != "" {
		if value := os.Getenv("WRITE_AFTER"); value != "" {
			seconds, err := strconv.Atoi(value)
			if err != nil {
				os.Exit(3)
			}
			time.Sleep(time.Duration(seconds) * time.Second)
		}
		if err := os.WriteFile(path, []byte(os.Getenv("WRITE_DATA")), 0o644); err != nil {
			os.Exit(2)
		}
	}
	if port := os.Getenv("HTTP"); port != "" {
		go func() {
			_ = http.ListenAndServe(":"+port, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				code := http.StatusOK
				if value := os.Getenv("HTTP_CODE"); value != "" {
					if parsed, err := strconv.Atoi(value); err == nil {
						code = parsed
					}
				}
				w.WriteHeader(code)
				_, _ = w.Write([]byte("ok"))
			}))
		}()
	}
	if path := os.Getenv("RECORD_SIGNAL"); path != "" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		go func() {
			for received := range signals {
				_ = os.WriteFile(path, []byte(received.String()), 0o644)
			}
		}()
	}
	if os.Getenv("IGNORE_TERM") == "true" {
		signal.Ignore(syscall.SIGTERM)
	}
	if health := os.Getenv("FIXTURE_HEALTH"); health != "" {
		if health == "fail" {
			os.Exit(1)
		}
		if health == "starting" {
			time.Sleep(2 * time.Minute)
		}
	}
	if value := os.Getenv("EXIT_AFTER"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil {
			os.Exit(3)
		}
		time.Sleep(time.Duration(seconds) * time.Second)
		fmt.Println("exit")
		os.Exit(0)
	}
	for {
		time.Sleep(time.Hour)
	}
}
