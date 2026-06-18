package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

const applicationVersion string = "v1.3.1"
const buildDateTime string = ""

var currentLogLevel string = "notice"
var dg *discordgo.Session // Global Discord session
var discordConnected bool = false
var timezone *time.Location

func init() {
	configfileflag := flag.String("config", "config.yaml", "Configuration file: /path/to/file.yaml, default = ./config.yaml")
	versionflag := flag.Bool("version", false, "Display version")
	helpflag := flag.Bool("help", false, "Display help")
	displayconfigflag := flag.Bool("displayconfig", false, "Display configuration")

	flag.Parse()

	if *versionflag {
		fmt.Printf("simple-discord-bot Version: %s Built: %s\n", applicationVersion, buildDateTime)
		os.Exit(0)
	}

	if *helpflag {
		displayHelp()
		os.Exit(0)
	}

	configdir, configfile := filepath.Split(*configfileflag)

	// set default configuration directory to current directory
	if configdir == "" {
		configdir = "."
	}

	viper.SetConfigType("yaml")
	viper.AddConfigPath(configdir)

	config := strings.TrimSuffix(configfile, ".yaml")
	config = strings.TrimSuffix(config, ".yml")

	viper.SetConfigName(config)

	loadConfig()

	if *displayconfigflag {
		displayConfig()
		os.Exit(0)
	}

	if !viper.IsSet("_discord_token") {
		logger("emergency", "No _discord_token configured")
	}

	if !viper.IsSet("_log_level") {
		currentLogLevel = "notice"
	} else {
		currentLogLevel = viper.GetString("_log_level")
	}

}

func main() {

	// Set the timezone
	var err error
	configTimeZone := viper.GetString("_timezone")
	timezone, err = time.LoadLocation(configTimeZone) // Replace with your desired timezone
	if err != nil {
		fmt.Println("Error loading timezone:", err)
		return
	}

	// Initialize Discord bot and check for errors
	if err := initDiscord(); err != nil {
		logger("emergency", "Failed to start Discord bot: %s", err)
	}

	if viper.GetBool("_canary_enabled") {
		go canaryCheckin(viper.GetString("_canary_url"), viper.GetInt("_canary_interval"))
	}

	if viper.GetBool("_shell_enabled") && !viper.IsSet("_shell") {
		logger("emergency", "If _shell_enabled=true, then _shell must be defined")
		os.Exit(1)
	}

	initHTTPListener()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}
