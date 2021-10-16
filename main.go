package main

import (
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const applicationVersion string = "v0.1"

var (
	Token string
	Color = 0x009688
	//Icons = "https://kittyhacker101.tk/Static/KatBot"
	Icons    = "https://cdn.discordapp.com/emojis"
	Emojis   = make(map[string]string)
	Channels = make(map[string]string)
)

func init() {
	flag.String("config", "config.yaml", "Configuration file: /path/to/file.yaml, default = ./config.yaml")
	flag.Bool("help", false, "Display help information")
	flag.Bool("displayconfig", false, "Display configuration")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	if viper.GetBool("help") {
		displayHelp()
		os.Exit(0)
	}

	configdir, configfile := filepath.Split(viper.GetString("config"))

	// set default configuration directory to current directory
	if configdir == "" {
		configdir = "."
	}

	viper.SetConfigType("yaml")
	viper.AddConfigPath(configdir)

	config := strings.TrimSuffix(configfile, ".yaml")
	config = strings.TrimSuffix(config, ".yml")

	viper.SetConfigName(config)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Fatal("Config file not found")
		} else {
			log.Fatal("Config file was found but another error was discovered")
		}
	}

	Token = viper.GetString("discordtoken")
	fmt.Println("discordtoken=", Token)
}

func main() {
	if viper.GetBool("displayconfig") {
		displayConfig()
		os.Exit(0)
	}

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func displayConfig() {
	allmysettings := viper.AllSettings()
	var keys []string
	for k := range allmysettings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println("CONFIG:", k, ":", allmysettings[k])
	}
}

func displayHelp() {
	message := `
      --config string       Configuration file: /path/to/file.yaml (default "./config.yaml")
      --displayconfig       Display configuration
      --help                Display help information
`
	fmt.Println("simple-discord-bot " + applicationVersion)
	fmt.Println(message)
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, viper.GetString("commandkey")+" ") {
		fmt.Println("not a command we care about")
		return
	}

	cleancommand := strings.Replace(m.Content, viper.GetString("commandkey")+" ", "", 1)
	fmt.Println("cleancommand=", cleancommand)

	if cleancommand == "help" {
		s.ChannelMessageSend(m.ChannelID, viper.GetString("discordhelp"))
		return
	}

	if val, ok := viper.GetStringMap("commands")[cleancommand]; ok {
		fmt.Println("val=", val)
		s.ChannelMessageSend(m.ChannelID, viper.GetStringMap("commands")[cleancommand].(string))
		return
	}

	if strings.HasPrefix(cleancommand, "camera ") {

		parts := strings.Split(cleancommand, " ")

		// list cameras
		if parts[1] == "list" {
			// print the cameras
			cameralist := viper.GetStringSlice("cameras")
			fmt.Printf("Type: %T,  Size: %d \n", cameralist, len(cameralist))
			if len(cameralist) > 0 {
				printtext := "```\n"
				for _, camera := range cameralist {
					printtext = printtext + camera + "\n"
				}
				printtext = printtext + "```"
				s.ChannelMessageSend(m.ChannelID, printtext)
				return
			} else {
				s.ChannelMessageSend(m.ChannelID, "```No cameras found```")
				return
			}
		}

		// take snapshot
		if parts[1] == "snapshot" {

			s.ChannelMessageSend(m.ChannelID, viper.GetString("cameraurl")+"/"+takeSnapshot(parts[2]))

		}

	}

	if m.Content == "!cat" {
		tr := &http.Transport{DisableKeepAlives: true}
		client := &http.Client{Transport: tr}
		resp, err := client.Get("https://images-na.ssl-images-amazon.com/images/I/71FcdrSeKlL._AC_SL1001_.jpg")
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Unable to fetch cat!")
			fmt.Println("[Warning] : Cat API Error")
		} else {
			s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{Name: "Cat Picture", IconURL: Icons + "/729726642758615151.png"},
				Color:  Color,
				Image: &discordgo.MessageEmbedImage{
					URL: resp.Request.URL.String(),
				},
				Footer: &discordgo.MessageEmbedFooter{Text: "Cat pictures provided by TheCatApi", IconURL: Icons + "/729726642758615151.png"},
			})
			fmt.Println("[Info] : Cat sent successfully to " + m.Author.Username + "(" + m.Author.ID + ") in " + m.ChannelID)
		}
	}
}

func someMessage(message string) string {
	return message
}

func takeSnapshot(camera string) string {
	fmt.Println("takesnapshot=", camera)
	url := viper.GetString("cameraserver") + "/snap?camera=" + camera
	fmt.Println("url=", url)
	resp, err := http.Get(url)
	if err != nil {
		return "cannot take snapshot"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {

		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			log.Println("Error: Could not take snapshot " + url)
			return "Could not take snapshot"
		}

		return string(body)
	} else {
		return "Could not take snapshot"
	}
}
