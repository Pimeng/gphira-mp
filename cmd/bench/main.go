package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type globalArgs struct {
	host string
	port int
}

func parseGlobalArgs(fs *flag.FlagSet, args *globalArgs) {
	fs.StringVar(&args.host, "host", "127.0.0.1", "Server host")
	fs.IntVar(&args.port, "port", 12346, "Server port")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	sub := os.Args[1]
	switch sub {
	case "connect":
		runConnect(os.Args[2:])
	case "room":
		runRoom(os.Args[2:])
	case "gameplay":
		runGameplay(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", sub)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: bench <subcommand> [options]

Subcommands:
  connect    Connection benchmark
  room       Room creation/join benchmark
  gameplay   Gameplay load benchmark

Global options (available for all subcommands):
  --host     Server host (default: 127.0.0.1)
  --port     Server port (default: 12346)

Use "bench <subcommand> --help" for subcommand-specific options.`)
}

func splitTokens(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
