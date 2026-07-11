package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ShizukaJiku/gameops/backup"
	"github.com/ShizukaJiku/gameops/idlewatch"
	"github.com/ShizukaJiku/gameops/internal/config"
	"github.com/ShizukaJiku/gameops/maintenance"
	"github.com/ShizukaJiku/gameops/startup"
)

func main() {
	// Every subcommand here typically runs from a Scheduled Task with no
	// console attached, so the default stderr output goes nowhere — persist
	// it to a file next to the binary so decisions (idle-stop, backup
	// failures, maintenance actions) are auditable after the fact.
	logFile, err := os.OpenFile("gameops.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "idle-watch":
		runIdleWatch(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	case "maintenance":
		runMaintenance(os.Args[2:])
	case "startup":
		runStartup(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gameops <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  idle-watch          run the idle-watch daemon (auto stop/start backend on inactivity)")
	fmt.Fprintln(os.Stderr, "  backup run          back up all configured instances' worlds once and exit")
	fmt.Fprintln(os.Stderr, "  maintenance stop    stop all configured instances (clean RCON stop, force-kill fallback)")
	fmt.Fprintln(os.Stderr, "  maintenance resume  restart all configured instances after maintenance")
	fmt.Fprintln(os.Stderr, "  startup apply       apply configured startup commands to all instances via RCON")
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

func runStartup(args []string) {
	if len(args) < 1 || args[0] != "apply" {
		usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("startup apply", flag.ExitOnError)
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
		if err := startup.Apply(instCfg); err != nil {
			log.Printf("[%s] startup apply failed: %v", instCfg.Name, err)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}

func runMaintenance(args []string) {
	if len(args) < 1 || (args[0] != "stop" && args[0] != "resume") {
		usage()
		os.Exit(1)
	}
	action := args[0]

	fs := flag.NewFlagSet("maintenance "+action, flag.ExitOnError)
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
		var actionErr error
		if action == "stop" {
			actionErr = maintenance.Stop(instCfg)
		} else {
			actionErr = maintenance.Resume(instCfg)
		}
		if actionErr != nil {
			log.Printf("[%s] maintenance %s failed: %v", instCfg.Name, action, actionErr)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}
