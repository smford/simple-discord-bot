package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const applicationVersion string = "v0.5.6"

var (
	Token string
)

func init() {
	flag.String("config", "config.yaml", "Configuration file: /path/to/file.yaml, default = ./config.yaml")
	flag.Bool("displayconfig", false, "Display configuration")
	flag.Bool("help", false, "Display help")
	flag.Bool("version", false, "Display version")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	if viper.GetBool("help") {
		displayHelp()
		os.Exit(0)
	}

	if viper.GetBool("version") {
		fmt.Printf("simple-discord-bot %s\n", applicationVersion)
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
			log.Fatal("Config file was found but another error was discovered: ", err)
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
      --help                Display help
      --version             Display version
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

	// strip out the command key
	cleancommand := strings.Replace(strings.ToLower(m.Content), viper.GetString("commandkey")+" ", "", 1)

	// mycommand = command
	// iscommandvalid = is command valid
	// myoptions = map of all options, ready for templating
	mycommand, iscommandvalid, myoptions := findCommand(cleancommand)

	fmt.Printf("findcommandreturn: command=%s, isValid?=%b, options=%v\n", mycommand, iscommandvalid, myoptions)

	if !iscommandvalid {
		fmt.Println("Command:\"%s\" is not valid, ignoring\n", mycommand)
		return
	}

	fmt.Printf("Number of options=%d\n", len(myoptions))

	if _, ok := viper.GetStringMap("commands")[mycommand]; ok {
		fmt.Printf("command=%s        action=%s\n", mycommand, viper.GetStringMap("commands")[mycommand])
	}

	aftertemplate := viper.GetStringMap("commands")[mycommand]
	fmt.Printf("before template replacement: \"%s\"\n", aftertemplate)
	for key, value := range myoptions {

		aftertemplate = strings.Replace(aftertemplate.(string), key, value, -1)

	}

	fmt.Printf(" After template replacement: \"%s\"\n", aftertemplate)

	// find role for the primary command
	commandrole := getCommandRole(mycommand)

	// check if a role has been assigned to the command, and ignore if none has been set or role is invalid
	if !isRoleValid(commandrole) {
		// role doesn't exist
		log.Printf("Error: commandrole doesnt exist for %s", mycommand)
		return
	}

	// check if user has permissions to execute a command
	if !checkUserPerms(commandrole, author, m.Author.ID) {
		log.Printf("Error: User:%s ID:%s Does not have permission to run Command: \"%s\"\n", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// check if command is valid and do appropriate text response
	if _, ok := viper.GetStringMap("commands")[mycommand]; ok {

		//commandmessageparts := strings.Split(viper.GetStringMap("commands")[mycommand].(string), "|")
		commandmessageparts := strings.Split(aftertemplate.(string), "|")

		issecret := false
		isapicall := false
		isfile := false
		//istemplate := false

		var messagetosend string

		// discern whether command is an apicall or secret
		for _, value := range commandmessageparts {
			if value == "secret" {
				issecret = true
			}
			if value == "api" {
				isapicall = true
			}
			if value == "file" {
				isfile = true
			}
			//if value == "template" {
			//	istemplate = true
			//}
		}

		// if api and file then return and throw an error, this is not a valid option configuration
		if isapicall && isfile {
			log.Printf("Error: Cannot have command api| with file| on command %s\n", mycommand)
			return
		}

		// strip "api|", "file|", "template|"  and "secret|" from the command
		messagetosend = strings.Replace(aftertemplate.(string), "api|", "", -1)
		messagetosend = strings.Replace(messagetosend, "file|", "", -1)
		//messagetosend = strings.Replace(messagetosend, "template|", "", -1)
		messagetosend = strings.Replace(messagetosend, "secret|", "", -1)

		// if an api call do it and get response which will become the message sent to the user
		if isapicall {
			messagetosend = downloadApi(messagetosend)
		}

		// if we need to load a files contents into message to send
		if isfile {
			tempcontents, err := loadFile(messagetosend)
			if err != nil {
				log.Printf("Error loading file: %s with: %s\n", messagetosend, err)
				return
			}

			messagetosend = tempcontents
		}

		// send the command response, if marked as secret send via private message
		if issecret {
			privateMessageCreate(s, m.Author.ID, messagetosend)
		} else {
			s.ChannelMessageSend(m.ChannelID, messagetosend)
		}

		return
	}

	/*
		// handle camera related commands
		if cleancommandparts[1] == "camera" {

			// list cameras
			if cleancommandparts[2] == "list" {

				cameralisturl := viper.GetString("cameraserver") + "/cameras?json=y"

				cameralist := downloadApi(cameralisturl)

				fmt.Printf("cameralist=%s\n", cameralist)

				s.ChannelMessageSend(m.ChannelID, cameralist)

				return
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
	*/
}

// make a query to a url
func downloadApi(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error: Could not connect to api url:\"%s\" with error:%s", url, err)
		return "error"
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

func loadFile(filename string) (string, error) {
	// clean file name to prevent path traversal
	cleanFilename := path.Join("/", filename)

	// load the file
	filecontents, err := ioutil.ReadFile(cleanFilename)

	// return contents and any error
	return string(filecontents), err
}

func findCommand(thecommand string) (string, bool, map[string]string) {

	isValidCommand := false

	allparts := strings.Split(thecommand, " ")
	num_allparts := len(allparts)

	var checkthiscommand string = ""

	var lastvalidcommandfound string = ""

	//var command_num int = 0
	//var optional_num int = 0

	var option_num int = 0
	//var options map[string]string

	options := make(map[string]string)

	for i := 0; i < num_allparts; i++ {
		fmt.Printf("%d =================\n", i)
		if i == 0 {
			checkthiscommand = allparts[0]
		} else {
			checkthiscommand = checkthiscommand + " " + allparts[i]
		}

		fmt.Println("findCommand: checking " + checkthiscommand)
		if _, ok := viper.GetStringMap("commands")[checkthiscommand]; ok {
			fmt.Println("FOUND COMMAND")
			lastvalidcommandfound = checkthiscommand
			isValidCommand = true

			// assume all remaining unparse tokens are optional.  each loop will update the list until no further valid commands are found

			option_num = 0
			new_options := make(map[string]string)
			for oi := i + 1; oi < num_allparts; oi++ {
				fmt.Printf("oi=%d  allparts[%d]=%s\n", oi, oi, allparts[oi])
				fmt.Println("setting new_options")
				new_options["{"+strconv.Itoa(option_num)+"}"] = allparts[oi]
				option_num++
			}

			fmt.Println("before options = new_options")
			options = new_options
			fmt.Println("after options = new_options")

		} else {
			fmt.Println("NOT FOUND COMMAND")
		}

	}

	for key, value := range options { // Order not specified
		fmt.Printf("options: key=%s value=%s\n", key, value)
	}

	return lastvalidcommandfound, isValidCommand, options
}
