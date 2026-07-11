package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ShizukaJiku/gameops/backup"
	"github.com/ShizukaJiku/gameops/idlewatch"
	"github.com/ShizukaJiku/gameops/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "idle-watch":
		runIdleWatch(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gameops <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  idle-watch    run the idle-watch daemon (auto stop/start backend on inactivity)")
	fmt.Fprintln(os.Stderr, "  backup run    back up all configured instances' worlds once and exit")
}

func runIdleWatch(args []string) {
	fs := flag.NewFlagSet("idle-watch", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	done := make(chan error, len(cfg.Instances))
	for _, instCfg := range cfg.Instances {
		instCfg := instCfg
		adapter, err := idlewatch.BuildAdapter(instCfg)
		if err != nil {
			log.Fatalf("[%s] %v", instCfg.Name, err)
		}
		in := idlewatch.NewInstance(instCfg, adapter)
		go func() { done <- in.Run() }()
	}

	err = <-done
	log.Fatalf("an instance stopped unexpectedly: %v", err)
}

func runBackup(args []string) {
	if len(args) < 1 || args[0] != "run" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("backup run", flag.ExitOnError)
	configPath := fs.String("config", "gameops.toml", "path to config file")
	fs.Parse(args[1:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config %s: %v", *configPath, err)
	}
	if len(cfg.Instances) == 0 {
		log.Fatal("config has no [[instances]] defined")
	}

	failed := false
	for _, instCfg := range cfg.Instances {
		if instCfg.Backup == nil {
			continue
		}
		path, err := backup.Run(instCfg)
		if err != nil {
			log.Printf("[%s] backup failed: %v", instCfg.Name, err)
			failed = true
			continue
		}
		if path != "" {
			log.Printf("[%s] backup complete: %s", instCfg.Name, path)
		}
	}

	if failed {
		os.Exit(1)
	}
}
