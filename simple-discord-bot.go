package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const applicationVersion string = "v0.6"

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
		log.Println("error creating Discord session,", err)
		return
	}

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		log.Println("error opening connection,", err)
		return
	}

	if viper.GetBool("canaryenable") {
		go canaryCheckin(viper.GetString("canaryurl"), viper.GetInt("canaryinterval"))
	}

	if viper.GetBool("shellenable") && !viper.IsSet("shell") {
		log.Println("Error: If shellenable=true, a shell must be defined")
		os.Exit(1)
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
	log.Printf("User:%s ID:%s Command:\"%s\"\n", m.Author.Username, m.Author.ID, m.Content)

	// strip out the command key
	cleancommand := strings.Replace(strings.ToLower(m.Content), viper.GetString("commandkey")+" ", "", 1)

	// mycommand = the valid command found
	// iscommandvalid = is command valid?
	// myoptions = map of all options, ready for templating
	mycommand, iscommandvalid, myoptions := findCommand(cleancommand)

	if !iscommandvalid {
		log.Printf("User:%s ID:%s Command:\"%s\" Status:\"Command is invalid\"\n", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	aftertemplate := viper.GetStringMap("commands")[mycommand]

	// do all the templating, replace {0} etc in the command with the options the user has given
	for key, value := range myoptions {
		aftertemplate = strings.Replace(aftertemplate.(string), key, value, -1)
	}

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

		commandmessageparts := strings.Split(aftertemplate.(string), "|")

		issecret := false
		isapicall := false
		isfile := false
		isshell := false

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
			if value == "shell" {
				isshell = true
			}
		}

		// if api and file then return and throw an error, this is not a valid option configuration
		if isapicall && isfile {
			log.Printf("Error: Cannot have command api| with file| on command %s\n", mycommand)
			return
		}

		// if shell and (file or api) then return and throw an error, this is not a valid option configuration
		if isshell && (isfile || isapicall) {
			log.Printf("Error: Cannot have command shell| with file| or api| on command %s\n", mycommand)
			return
		}

		// strip "api|", "file|" and "secret|" from the commands action
		messagetosend = strings.Replace(aftertemplate.(string), "api|", "", -1)
		messagetosend = strings.Replace(messagetosend, "file|", "", -1)
		messagetosend = strings.Replace(messagetosend, "secret|", "", -1)
		messagetosend = strings.Replace(messagetosend, "shell|", "", -1)

		// if an api call do it and get response which will become the message sent to the user
		if isapicall {
			messagetosend = downloadApi(messagetosend)
		}

		// if we need to load a files contents into message to send
		if isfile {
			tempcontents, err := loadFile(messagetosend)
			if err != nil {
				log.Printf("Error loading file: %s with: %v\n", messagetosend, err)
				return
			}

			messagetosend = tempcontents
		}

		if isshell && viper.GetBool("shellenable") {
			err, stdout, stderr := shellOut(messagetosend)
			if err != nil {
				log.Printf("Error: Error executing command:\"%s\" err:%v\n", messagetosend, err)
			}

			messagetosend = "```\n"
			if len(stdout) > 0 {
				messagetosend = messagetosend + stdout
			}
			if len(stderr) > 0 {
				messagetosend = messagetosend + "\nSTDERR:\n-------\n" + stderr
			}
			messagetosend = messagetosend + "```\n"

			// if messagetosend is empty, do nothing and return
			if len(messagetosend) == 8 {
				return
			}
		}

		// do nothing and return when command is a shell and shellenable = false
		if isshell && !viper.GetBool("shellenable") {
			log.Println("Error: Cannot run shell command when shellenable = false")
			return
		}

		// send the command response, if marked as secret send via private message
		if issecret {
			privateMessageCreate(s, m.Author.ID, messagetosend)
		} else {
			s.ChannelMessageSend(m.ChannelID, messagetosend)
		}

		return
	}
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

	var option_num int = 0

	options := make(map[string]string)

	for i := 0; i < num_allparts; i++ {
		if i == 0 {
			checkthiscommand = allparts[0]
		} else {
			checkthiscommand = checkthiscommand + " " + allparts[i]
		}

		if _, ok := viper.GetStringMap("commands")[checkthiscommand]; ok {
			lastvalidcommandfound = checkthiscommand
			isValidCommand = true

			// assume all remaining unparse tokens are optional.  each loop will update the list until no further valid commands are found
			option_num = 0
			new_options := make(map[string]string)
			for oi := i + 1; oi < num_allparts; oi++ {
				new_options["{"+strconv.Itoa(option_num)+"}"] = allparts[oi]
				option_num++
			}

			options = new_options

		} else {
			// command not matched, continue iterating through commands looking for the longest matching combination
		}

	}

	return lastvalidcommandfound, isValidCommand, options
}

func canaryCheckin(url string, interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	for _ = range ticker.C {
		client := http.Client{
			Timeout: 5 * time.Second,
		}
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("Error: Could not connect to canary with error:%s", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Printf("Error: Could not checkin to canary with error:%s", err)
			} else {
				log.Println("Success: Canary Checkin")
			}
		}
	}
}

func shellOut(command string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(viper.GetString("shell"), "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return err, stdout.String(), stderr.String()
}
