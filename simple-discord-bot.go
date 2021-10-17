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
	"strconv"
	"strings"
	"syscall"
)

const applicationVersion string = "v0.3"

var (
	Token string
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

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	fmt.Println("simple-discord-bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	dg.Close()
}

// displays configuration
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

// displays help information
func displayHelp() {
	message := `
      --config string       Configuration file: /path/to/file.yaml (default "./config.yaml")
      --displayconfig       Display configuration
      --help                Display help information
`
	fmt.Println("simple-discord-bot " + applicationVersion)
	fmt.Println(message)
}

// discord message handler
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// ignore messages from itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	fmt.Printf("m=%+v\n", m)
	fmt.Printf("s.State.User.ID type=%T\n", s.State.User.ID)
	fmt.Println("s.State.User.ID=", s.State.User.ID)
	fmt.Printf("m.author.id type=%T\n", m.Author.ID)
	fmt.Println("m.author.id=", m.Author.ID)

	listRoles()

	// ignore commands we don't care about
	if !strings.HasPrefix(m.Content, viper.GetString("commandkey")+" ") {
		return
	}

	// clean up the message/command
	cleancommand := strings.Replace(m.Content, viper.GetString("commandkey")+" ", "", 1)

	commandrole := getCommandRole(cleancommand)

	fmt.Printf("found commandrole=%s\n", commandrole)

	if !isRoleValid(commandrole) {
		// role doesn't exist
		log.Printf("Error commandrole doesnt exist for %s", cleancommand)
		return
	}

	fmt.Println("getcommandrole=", commandrole)

	if checkUserPerms(commandrole, m.Author.ID) {
		fmt.Printf("User: %s has role: %s that command: %s requires\n", m.Author.ID, commandrole, cleancommand)

	} else {
		fmt.Printf("User: %s does not have role: %s that command: %s requires\n", m.Author.ID, commandrole, cleancommand)

	}

	// display help information
	if cleancommand == "help" {
		s.ChannelMessageSend(m.ChannelID, viper.GetString("discordhelp"))
		return
	}

	// check if command is valid and do appropriate simple text response
	if _, ok := viper.GetStringMap("commands")[cleancommand]; ok {
		s.ChannelMessageSend(m.ChannelID, viper.GetStringMap("commands")[cleancommand].(string))
		return
	}

	// handle camera related commands
	if strings.HasPrefix(cleancommand, "camera ") {

		parts := strings.Split(cleancommand, " ")

		// list cameras
		if parts[1] == "list" {
			cameralist := viper.GetStringSlice("cameras")
			sort.Strings(cameralist)
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

			// check that camera given in message/command is valid
			if foundCamera(parts[2]) {

				// take a snapshot
				snapshotresult := takeSnapshot(parts[2])

				// check that return message is valid
				if strings.HasPrefix(snapshotresult, "files/") {
					// display link to image
					s.ChannelMessageSend(m.ChannelID, viper.GetString("cameraurl")+"/"+snapshotresult)
				} else {
					// display error message from motioneye-snapshotter
					s.ChannelMessageSend(m.ChannelID, snapshotresult)
				}

				// camera is not valid
			} else {
				s.ChannelMessageSend(m.ChannelID, "Unknown camera")
			}

		}

	}
}

// take a snapshot of the camera using motioneye-snapshotter
func takeSnapshot(camera string) string {
	url := viper.GetString("cameraserver") + "/snap?camera=" + camera
	resp, err := http.Get(url)
	if err != nil {
		return "Could not take snapshot"
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
		log.Println("Error: Could not take snapshot " + url + " HTTPStatus: " + string(resp.StatusCode))
		return "Could not take snapshot"
	}
}

// check whether camera is valid
func foundCamera(camera string) bool {
	for _, result := range viper.GetStringSlice("cameras") {
		if result == camera {
			return true
		}
	}
	return false
}

// tells us what role a command requires
func getCommandRole(command string) string {
	if viper.IsSet("commandperms") {
		if _, ok := viper.GetStringMap("commandperms")[command]; ok {
			return viper.GetStringMap("commandperms")[command].(string)
		}
	}
	return "no role set"
}

// check if a user has a particular role, if they have a role return true
func checkUserPerms(role string, userid string) bool {
	fmt.Println("===checkUserPerms")

	if role == "no role set" {
		fmt.Println("no role set, permission denied")
		return false
	}

	if role == "all" {
		// everyones allowed to run this command
		return true
	}

	fmt.Printf("role=%s    user=%s\n", role, userid)

	result := viper.GetStringMap("commandroles")

	for _, users := range result[role].([]interface{}) {
		fmt.Printf(" - %s    type: %T    converted: %T\n", users, users, strconv.Itoa(users.(int)))
	}

	// func sliceContains(s []string, str string) bool {
	// if sliceContains(result[role].([]interface{}), userid) {
	if sliceContains(result[role].([]interface{}), userid) {
		//if sliceContains(users, userid) {
		fmt.Printf("Found user %s in %s!\n", userid, role)
		return true
	} else {
		fmt.Println("not here")
	}

	fmt.Println("=================")

	return false
}

func listRoles() {
	//viper.GetStringMap("commands")[cleancommand].(string)
	fmt.Printf("commandroles=%+v\n", viper.GetStringMap("commandroles"))
	fmt.Printf("commandroles type=%T\n", viper.GetStringMap("commandroles"))

	fmt.Println("listRoles====")
	for k, v := range viper.GetStringMap("commandroles") {
		fmt.Printf("k=%s      v=%T\n", k, v)

		for _, user := range v.([]interface{}) {
			fmt.Println(" - ", user)
		}

	}
	fmt.Println("=============")
}

// checks if a role is valid
func isRoleValid(role string) bool {

	if strings.ToLower(role) == "all" {
		return true
	}

	if viper.IsSet("commandroles") {
		fmt.Println("isset commandroles")
		if _, ok := viper.GetStringMap("commandroles")[role]; ok {
			fmt.Println("command roles found")
			return true
		} else {
			fmt.Println("command roles not found")
		}
	}

	fmt.Println("command perms fall through")
	return false
}

// does a int slice contain a value
// https://freshman.tech/snippets/go/check-if-slice-contains-element/
func sliceContains(i []interface{}, str string) bool {
	fmt.Println("sliceContains start")
	for _, v := range i {
		if strconv.Itoa(v.(int)) == str {
			return true
		}
	}
	return false
}
