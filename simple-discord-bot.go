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

const applicationVersion string = "v0.5.1"

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

	if !viper.IsSet("discordtoken") {
		log.Fatal("No discordtoken configured")
	}

	Token = viper.GetString("discordtoken")

	// listRoles()
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

	log.Printf("simple-discord-bot %s is now running.  Press CTRL-C to exit.\n", applicationVersion)
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

	chanl, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	guild, _ := s.Guild(chanl.GuildID)
	var author *discordgo.Member

	if guild != nil {
		author, _ = s.GuildMember(guild.ID, m.Author.ID)
	}

	// ignore commands we don't care about
	if !strings.HasPrefix(strings.ToLower(m.Content), strings.ToLower(viper.GetString("commandkey"))+" ") {
		return
	}

	// log commands passed to bot
	log.Printf("User:%s ID:%s Command: \"%s\"\n", m.Author.Username, m.Author.ID, m.Content)

	// break command up in to tokens
	cleancommandparts := strings.Split(strings.ToLower(m.Content), " ")

	// find role for the primary command
	commandrole := getCommandRole(cleancommandparts[1])

	// check if a role has been assigned to the command, and ignore if none has been set or role is invalid
	if !isRoleValid(commandrole) {
		// role doesn't exist
		log.Printf("Error: commandrole doesnt exist for %s", cleancommandparts[1])
		return
	}

	// check if user has permissions to execute a command
	if !checkUserPerms(commandrole, author, m.Author.ID) {
		log.Printf("Error: User:%s ID:%s Does not have permission to run Command: \"%s\"\n", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// display help information and return
	if cleancommandparts[1] == "help" {
		s.ChannelMessageSend(m.ChannelID, viper.GetString("discordhelp"))
		return
	}

	// check if command is valid and do appropriate simple text response
	if _, ok := viper.GetStringMap("commands")[cleancommandparts[1]]; ok {

		commandmessageparts := strings.Split(viper.GetStringMap("commands")[cleancommandparts[1]].(string), ":")

		// send the command response, if marked as secret send via private message
		if strings.ToLower(commandmessageparts[0]) == "secret" {
			privateMessageCreate(s, m.Author.ID, strings.Replace(viper.GetStringMap("commands")[cleancommandparts[1]].(string), "secret:", "", 1))
		} else {
			s.ChannelMessageSend(m.ChannelID, viper.GetStringMap("commands")[cleancommandparts[1]].(string))
		}

		return
	}

	// handle camera related commands
	if cleancommandparts[1] == "camera" {

		// list cameras
		if cleancommandparts[2] == "list" {
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
		if cleancommandparts[2] == "snapshot" {

			// check that camera given in message/command is valid
			if foundCamera(cleancommandparts[3]) {

				// take a snapshot
				snapshotresult := takeSnapshot(cleancommandparts[3])

				// check that return message is valid
				if strings.HasPrefix(snapshotresult, "files/") {
					// display link to image
					s.ChannelMessageSend(m.ChannelID, viper.GetString("cameraurl")+"/"+snapshotresult)
					log.Printf("User:%s ID:%s Snapshot: \"%s\"\n", m.Author.Username, m.Author.ID, viper.GetString("cameraurl")+"/"+snapshotresult)
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
		log.Printf("Error: Cannot execute Get snapshot for \"%s\", Message:%s\n", camera, err)
		return "Could not take snapshot"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			log.Printf("Error: Cannot take snapshot URL \"%s\", Message:%s\n", url, err)
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
func checkUserPerms(role string, user *discordgo.Member, userid string) bool {

	roledetails := strings.Split(strings.ToLower(role), ":")

	if roledetails[0] == "no role set" {
		// no role set, permission denied
		return false
	}

	if roledetails[0] == "all" {
		// everyones allowed to run this command
		return true
	}

	if roledetails[0] == "discord" {
		// check if users allowed via discord roles

		usersDiscordRoles := user.Roles

		for _, v := range usersDiscordRoles {
			if v == strconv.Itoa(viper.GetStringMap("discordroles")[roledetails[1]].(int)) {
				// found users discord role
				return true
			}
		}

		// user does not have needed discord role
		return false

	} else {
		// check normal roles

		result := viper.GetStringMap("commandroles")

		if sliceContainsInt(result[role].([]interface{}), userid) {
			// user has a role
			return true
		}

	}
	return false
}

// list normal roles and the users
func listRoles() {
	for k, v := range viper.GetStringMap("commandroles") {
		fmt.Printf("Role:%s\n", k)
		for _, user := range v.([]interface{}) {
			fmt.Println(" - ", user)
		}
	}
}

// checks if a role is valid
func isRoleValid(role string) bool {

	if strings.ToLower(role) == "all" {
		return true
	}

	roledetails := strings.Split(strings.ToLower(role), ":")

	// check if it is a discord role
	if roledetails[0] == "discord" {
		if !viper.IsSet("discordroles") {
			log.Printf("Error: discordroles not configured")
			return false
		}

		if _, ok := viper.GetStringMap("discordroles")[roledetails[1]]; ok {
			// found valid discord role
			return true
		}

		// no valid discord role found
		return false
	}

	// check if normal role
	if viper.IsSet("commandroles") {
		if _, ok := viper.GetStringMap("commandroles")[roledetails[0]]; ok {
			// found valid role in commandroles
			return true
		}
	}

	// catch all deny
	return false
}

// does a int slice contain a value
// https://freshman.tech/snippets/go/check-if-slice-contains-element/
func sliceContainsInt(i []interface{}, str string) bool {
	for _, v := range i {
		if strconv.Itoa(v.(int)) == str {
			return true
		}
	}
	return false
}

// send a private message to a user
func privateMessageCreate(s *discordgo.Session, userid string, message string) {

	// create the private message channel to user
	channel, err := s.UserChannelCreate(userid)
	if err != nil {
		log.Printf("Error: Creating PM channel to %s with %s\n", userid, err)
		s.ChannelMessageSend(userid, "Something went wrong while sending the DM!")
		return
	}

	// send the message to the user
	_, err = s.ChannelMessageSend(channel.ID, message)
	if err != nil {
		log.Printf("Error: Cannot send DM to %s with %s\n", userid, err)
		s.ChannelMessageSend(userid, "Failed to send you a DM. Did you disable DM in your privacy settings?")
	}

}
