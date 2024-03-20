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

const applicationVersion string = "v0.7.2"

var (
	Token string
)

func init() {
	flag.String("config", "config.yaml", "Configuration file: /path/to/file.yaml, default = ./config.yaml")
	flag.Bool("displayconfig", false, "Display configuration")
	flag.Bool("help", false, "Display help")
	flag.Bool("version", false, "Display version")
	flag.Int("chunksize", 1980, "Message chunk size, default = 1980")
	flag.String("splitchar", "\n", "Character to split chunks on, default = \\n")

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

	dg.AddHandler(addReaction)

	dg.AddHandler(removeReaction)

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

	// check tracked reactions
	checkReactions(dg)

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
      --chunksize int       Message chunk size, default = 1980
      --config string       Configuration file: /path/to/file.yaml (default "./config.yaml")
      --displayconfig       Display configuration
      --help                Display help
      --splitchar string    Character to split chunks on, default = \n
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
	} else {
		author, _ = s.GuildMember(viper.GetString("defaultserverid"), m.Author.ID)
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
	// commandoptions = map of all options, ready for templating
	mycommand, iscommandvalid, commandoptions := findCommand(cleancommand)

	if !iscommandvalid {
		log.Printf("User:%s ID:%s Command:\"%s\" Status:\"Command is invalid\"\n", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// find role for the primary command
	commandRoles := viper.GetStringSlice("commands." + mycommand + ".roles")

	// check if a role has been assigned to the command, and ignore if none has been set or role is invalid
	for _, role := range commandRoles {
		if !isRoleValid(role) {
			// role doesn't exist
			log.Printf("Error: role (%s) not valid do not exist for command %s", role, mycommand)
			return
		}
	}

	// check if user has permission to execute a command
	var canRun bool = false
	for _, role := range commandRoles {
		if checkUserPerms(role, author, m.Author.ID) {
			canRun = true
		}
	}
	if !canRun {
		log.Printf("Error: User:%s ID:%s Does not have permission to run Command: \"%s\"\n", m.Author.Username, m.Author.ID, m.Content)
		return
	}

	// check if command is valid and do appropriate text response
	if _, ok := viper.GetStringMap("commands")[mycommand]; ok {

		ismessage := viper.IsSet("commands." + mycommand + ".message")
		isapicall := viper.IsSet("commands." + mycommand + ".api")
		isfile := viper.IsSet("commands." + mycommand + ".file")
		isshell := viper.IsSet("commands." + mycommand + ".shell")
		isfunction := viper.IsSet("commands." + mycommand + ".function")
		issecret := viper.GetBool("commands." + mycommand + ".secret")

		// if api and file then return and throw an error, this is not a valid option configuration
		if isapicall && isfile {
			log.Printf("Error: Cannot have command api with file on command %s\n", mycommand)
			return
		}

		// if shell and (file or api) then return and throw an error, this is not a valid option configuration
		if isshell && (isfile || isapicall) {
			log.Printf("Error: Cannot have command shell with file or api on command %s\n", mycommand)
			return
		}

		// if function and (file or api or shell) then return and throw an error, this is not a valid option configuration
		if isfunction && (isshell || isfile || isapicall) {
			log.Printf("Error: Cannot have command function with shell or file or api on command %s\n", mycommand)
			return
		}

		var messagetosend string

		if ismessage {
			messagetosend = prepareTemplate(viper.GetString("commands."+mycommand+".message"), commandoptions)
		} else if isapicall {
			// if an api call do it and get response which will become the message sent to the user
			messagetosend = downloadApi(prepareTemplate(viper.GetString("commands."+mycommand+".api"), commandoptions))

		} else if isfile {
			// if we need to load a files contents into message to send
			tempcontents, err := loadFile(prepareTemplate(viper.GetString("commands."+mycommand+".file"), commandoptions))
			if err != nil {
				log.Printf("Error loading file: %s with: %v\n", messagetosend, err)
				return
			}

			messagetosend = tempcontents
		} else if isshell && viper.GetBool("shellenable") {
			err, stdout, stderr := shellOut(prepareTemplate(viper.GetString("commands."+mycommand+".shell"), commandoptions))
			if err != nil {
				log.Printf("Error: Error executing command:\"%s\" err:%v\n", messagetosend, err)
			}

			messagetosend = ""
			if len(stdout) > 0 {
				messagetosend = messagetosend + stdout
			}
			if len(stderr) > 0 {
				messagetosend = messagetosend + "\nSTDERR:\n-------\n" + stderr
			}
			//messagetosend = messagetosend + "```\n"

			// if messagetosend is empty, do nothing and return
			if len(messagetosend) == 8 {
				return
			}
		} else if isshell && !viper.GetBool("shellenable") {
			// do nothing and return when command is a shell and shellenable = false
			log.Println("Error: Cannot run shell command when shellenable = false")
			return
		} else if isfunction {
			lengthOfMessageWithoutCommand := len(viper.GetString("commandkey")) + 1 + len(mycommand) + 1
			var message string
			if lengthOfMessageWithoutCommand > len(m.Content) {
				message = ""
			} else {
				message = m.Content[lengthOfMessageWithoutCommand:]
			}

			functionName := prepareTemplate(viper.GetString("commands."+mycommand+".function"), commandoptions)
			// Map function names to actual functions
			functions := map[string]func(*discordgo.Session, *discordgo.MessageCreate, string){
				"sendMessage": sendMessage,
				"editMessage": editMessage,
				"listEmoji":   listEmoji,
				"showHelp":    showHelp,
			}

			// Call the function based on the name
			if function, ok := functions[functionName]; ok {
				function(s, m, message)
			} else {
				fmt.Println("Function", functionName, "not found")
			}

		}

		var usewrapper = false

		if isshell || isfile {
			usewrapper = true
		}

		// send the command response, if marked as secret send via private message do not send if command is a custom function
		if !isfunction {
			if issecret {
				privateMessageCreate(s, m.Author.ID, messagetosend, usewrapper)
			} else {
				channelMessageCreate(s, m, messagetosend, usewrapper)
			}
		}

		return
	}
}

func prepareTemplate(message string, commandoptions map[string]string) string {
	// do all the templating, replace {0} etc in the command with the options the user has given
	for key, value := range commandoptions {
		message = strings.Replace(message, key, value, -1)
	}

	return message
}

// discord addReaction handler
func addReaction(s *discordgo.Session, mr *discordgo.MessageReactionAdd) {
	for _, v := range viper.GetStringMap("reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// add role
						s.GuildMemberRoleAdd(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			fmt.Println("Data is not a map[string]interface{}")
		}
	}
}

// discord removeReaction handler
func removeReaction(s *discordgo.Session, mr *discordgo.MessageReactionRemove) {
	for _, v := range viper.GetStringMap("reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// remove role
						s.GuildMemberRoleRemove(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			fmt.Println("Data is not a map[string]interface{}")
		}
	}
}

// check reactions
func checkReactions(s *discordgo.Session) {
	fmt.Println("Checking reactions for tracked messages")
	for _, v := range viper.GetStringMap("reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			channelID := strconv.Itoa(m["channel_id"].(int))
			messageID := strconv.Itoa(m["message_id"].(int))

			// check emoji is being tracked for this message
			messageReactions, err := s.MessageReactions(channelID, messageID, m["emoji"].(string), 100, "", "")
			if err != nil {
				log.Printf("Error: Checking reactions channelID:%s messageID:%s, Error:%s\n", channelID, messageID, err)
			}
			var hasBotReaction bool = false
			for _, user := range messageReactions {
				if user.ID == s.State.User.ID {
					hasBotReaction = true
				}
			}

			if !hasBotReaction {
				s.MessageReactionAdd(channelID, messageID, m["emoji"].(string))
				// pause to make sure reactions are added in order
				time.Sleep(1 * time.Second)
			}

		}
	}

}

// custom command function for sending messages as the bot
func sendMessage(s *discordgo.Session, m *discordgo.MessageCreate, content string) {

	// split the string by whitespace
	words := strings.Split(content, " ")

	// get channel ID
	channelID := strings.Join(words[0:1], " ")

	// Get the last words ignoring the first
	message := strings.Join(words[1:], " ")

	// send message to channel
	s.ChannelMessageSend(channelID, message)
}

// custom command function for editing messages as the bot
func editMessage(s *discordgo.Session, m *discordgo.MessageCreate, content string) {

	// split the string by whitespace
	words := strings.Split(content, " ")

	// get channel ID
	channelID := strings.Join(words[0:1], " ")

	// get message ID
	messageID := strings.Join(words[1:2], " ")

	// Get the last words ignoring the first two
	message := strings.Join(words[2:], " ")

	// edits message in channel
	s.ChannelMessageEdit(channelID, messageID, message)
}

// custom command function to list all Emoji
func listEmoji(s *discordgo.Session, m *discordgo.MessageCreate, content string) {

	words := strings.Split(content, " ")

	// get guild ID from message
	guildID := strings.Join(words[0:1], " ")

	//	var guildID string = m.GuildID

	if guildID == "" {
		guildID = m.GuildID
	}

	if guildID != "" {
		emojis, err := s.GuildEmojis(guildID)
		if err != nil {
			log.Printf("Error: could not get emoji with error:%s", err)
		}

		var message string

		for _, emoji := range emojis {
			if m.GuildID != "" {
				message += "<:" + emoji.Name + ":" + emoji.ID + ">  `" + emoji.ID + "    " + emoji.Name + "`\n"
			} else {
				message += emoji.ID + "    " + emoji.Name + "\n"
			}
		}

		if m.GuildID != "" {
			channelMessageCreate(s, m, "**Emoji for "+guildID+"**\n"+message, false)
		} else {
			privateMessageCreate(s, m.Author.ID, "**Emoji for "+guildID+"**\n```"+message+"```", false)
		}
	} else {
		if m.GuildID != "" {
			channelMessageCreate(s, m, "Guild/Server ID not found", false)
		} else {
			privateMessageCreate(s, m.Author.ID, "Guild/Server ID not found", false)
		}
	}
}

// custom command function to list all Emoji
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate, content string) {

	user, _ := s.GuildMember(viper.GetString("defaultserverid"), m.Author.ID)

	var helpMessage string

	commandkey := viper.GetString("commandkey")

	allCommands := viper.GetStringMap("commands")

	// Loop through the commands map
	for command, info := range allCommands {

		// check if user has permission to execute a command
		var canRun bool = false

		// Access the "roles" for each command
		roles, ok := info.(map[string]interface{})["roles"]
		if !ok {
			fmt.Printf("Help information not found for command %s\n", command)
			continue
		}

		for _, role := range roles.([]interface{}) {
			fmt.Println(role.(string))
			if checkUserPerms(role.(string), user, m.Author.ID) {
				canRun = true
			}
		}

		if canRun {
			// Access the "help" field for each command
			help, ok := info.(map[string]interface{})["help"].(string)
			if !ok {
				fmt.Printf("Help information not found for command %s\n", command)
				continue
			}

			helpMessage += commandkey + " " + command + strings.Repeat(" ", 30-len(command)) + "- " + help + "\n"
			fmt.Printf("Command: %s\nHelp: %s\n\n", command, help)
		}

	}

	// sort commands into alphabetical order

	// Split the string into lines
	lines := strings.Split(helpMessage, "\n")

	// Sort the lines alphabetically
	sort.Strings(lines)

	// Join the sorted lines back together
	helpMessage = strings.Join(lines, "\n")

	helpMessage = "Help Commands:\n--------------\n" + helpMessage

	privateMessageCreate(s, m.Author.ID, helpMessage, true)
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

		if user != nil {
			usersDiscordRoles := user.Roles

			for _, v := range usersDiscordRoles {
				if v == strconv.Itoa(viper.GetStringMap("discordroles")[roledetails[1]].(int)) {
					// found users discord role
					return true
				}
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
			// found valid role in permissions
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
func privateMessageCreate(s *discordgo.Session, userid string, message string, codeblock bool) {
	var wrapper string
	if codeblock {
		wrapper = "```"
	}

	// create the private message channel to user
	channel, err := s.UserChannelCreate(userid)
	if err != nil {
		log.Printf("Error: Creating PM channel to %s with %s\n", userid, err)
		s.ChannelMessageSend(userid, "Something went wrong while sending the DM!")
		return
	}

	if len(message) > viper.GetInt("chunksize") {
		messagechunks := chunkMessage(message, viper.GetString("splitchar"), viper.GetInt("chunksize"))

		var allkeys []int

		for k, _ := range messagechunks {
			allkeys = append(allkeys, k)
		}

		sort.Ints(allkeys[:])

		for _, key := range allkeys {
			_, err = s.ChannelMessageSend(channel.ID, wrapper+messagechunks[key]+wrapper)
			// todo: catch errors here
		}

	} else {
		// send the message to the user
		_, err = s.ChannelMessageSend(channel.ID, wrapper+message+wrapper)
		if err != nil {
			log.Printf("Error: Cannot send DM to %s with %s\n", userid, err)
			s.ChannelMessageSend(userid, "Failed to send you a DM. Did you disable DM in your privacy settings?")
		}
	}

}

// send a message to a channel
func channelMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate, message string, codeblock bool) {
	var wrapper string
	if codeblock {
		wrapper = "```"
	}

	var err error

	if len(message) > viper.GetInt("chunksize") {
		messagechunks := chunkMessage(message, viper.GetString("splitchar"), viper.GetInt("chunksize"))
		var allkeys []int
		for k, _ := range messagechunks {
			allkeys = append(allkeys, k)
		}
		sort.Ints(allkeys[:])

		for _, key := range allkeys {
			_, err = s.ChannelMessageSend(m.ChannelID, wrapper+messagechunks[key]+wrapper)
			// todo: handle error
		}

	} else {

		// send the message to the user
		_, err = s.ChannelMessageSend(m.ChannelID, wrapper+message+wrapper)
		if err != nil {
			log.Printf("Error: Cannot send message to channel: %s\n", err)
		}
	}

}

// reads a file
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

// runs a shell command and gathers output
func shellOut(command string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(viper.GetString("shell"), "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return err, stdout.String(), stderr.String()
}

// splits (chunks) a message
func chunkMessage(message string, delimchar string, max int) map[int]string {
	sS := 0
	finished := false
	n := 0
	messagemap := make(map[int]string)

	for finished == false {
		if sS >= len(message) {
			break
		}
		endpoint := sS + max
		if endpoint > len(message) {
			endpoint = len(message)
			messagemap[n] = message[sS:endpoint]
			finished = true
		}

		foundPos := lastFoundBetween(message, delimchar, sS, endpoint)

		// no newline found, so chunk and move on
		if foundPos == -1 {
			messagemap[n] = message[sS:endpoint]
			sS = endpoint
		} else {
			messagemap[n] = message[sS : foundPos+1]
			sS = foundPos + 1
		}
		if sS >= len(message) {
			sS = len(message)
		}
		n++
	}
	return messagemap
}

// find the last occurance of a string between a range
func lastFoundBetween(s, sep string, start int, end int) int {
	idx := strings.LastIndex(s[start:end], sep)
	if idx > -1 {
		idx += start
	}
	return idx
}
