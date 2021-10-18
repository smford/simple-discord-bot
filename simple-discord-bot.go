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

const applicationVersion string = "v0.3.1"

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

	fmt.Printf("guild type = %T\n", s.State.Guild)
	fmt.Printf("guild string = %s\n", s.State.Guild)
	fmt.Printf("state structure=%+v\n", s.State)
	fmt.Printf("m=%+v\n", m)
	fmt.Printf("s.State.User.ID type=%T\n", s.State.User.ID)
	fmt.Println("s.State.User.ID=", s.State.User.ID)
	fmt.Printf("m.author.id type=%T\n", m.Author.ID)
	fmt.Println("m.author.id=", m.Author.ID)

	chanl, err := s.Channel(m.ChannelID)
	if err != nil {
		return
	}

	guild, _ := s.Guild(chanl.GuildID)
	var author *discordgo.Member

	if guild != nil {
		author, _ = s.GuildMember(guild.ID, m.Author.ID)
	}

	fmt.Printf("author=%s\nauthor +v: %+v\nauthortype=%T\n", author, author, author)

	fmt.Printf("authorRoles=%s\nauthor +v: %+v\nauthortype=%T\n", author.Roles, author.Roles, author.Roles)

	//return

	listRoles()

	// ignore commands we don't care about
	if !strings.HasPrefix(strings.ToLower(m.Content), strings.ToLower(viper.GetString("commandkey"))+" ") {
		return
	}

	//fmt.Printf("Username=%s\n", m.Author.Username)
	log.Printf("User:%s ID:%s Command: \"%s\"\n", m.Author.Username, m.Author.ID, m.Content)

	// clean up the message/command
	//cleancommand := strings.Replace(m.Content, viper.GetString("commandkey")+" ", "", 1)

	cleancommandparts := strings.Split(strings.ToLower(m.Content), " ")

	//fmt.Println("cleancommand=", cleancommand)
	fmt.Println("cleancommandparts=", cleancommandparts)

	commandrole := getCommandRole(cleancommandparts[1])

	fmt.Printf("found commandrole=%s\n", commandrole)

	if !isRoleValid(commandrole) {
		// role doesn't exist
		log.Printf("Error commandrole doesnt exist for %s", cleancommandparts[1])
		return
	}

	fmt.Println("getcommandrole=", commandrole)

	//if checkUserPerms(commandrole, m.Author.ID) {
	if checkUserPerms(commandrole, author, m.Author.ID) {
		fmt.Printf("User: %s has role: %s that command: %s requires\n", m.Author.ID, commandrole, cleancommandparts[1])

	} else {
		fmt.Printf("User: %s does not have role: %s that command: %s requires\n", m.Author.ID, commandrole, cleancommandparts[1])
		return
	}

	// display help information and return
	if cleancommandparts[1] == "help" {
		s.ChannelMessageSend(m.ChannelID, viper.GetString("discordhelp"))
		return
	}

	// check if command is valid and do appropriate simple text response
	if _, ok := viper.GetStringMap("commands")[cleancommandparts[1]]; ok {
		s.ChannelMessageSend(m.ChannelID, viper.GetStringMap("commands")[cleancommandparts[1]].(string))
		return
	}

	// handle camera related commands
	//if strings.HasPrefix(cleancommand, "camera ") {
	if cleancommandparts[1] == "camera" {

		//parts := strings.Split(cleancommand, " ")

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
//func checkUserPerms(role string, userid string) bool {
func checkUserPerms(role string, user *discordgo.Member, userid string) bool {
	fmt.Println("===checkUserPerms")

	fmt.Printf("*** discord.Member=%s\n*** discord.Member type= %T\n*** discord.Member guts=%+v\n", user, user, user)

	roledetails := strings.Split(strings.ToLower(role), ":")
	fmt.Println("roledetails=", roledetails)

	if roledetails[0] == "no role set" {
		fmt.Println("no role set, permission denied")
		return false
	}

	if roledetails[0] == "all" {
		// everyones allowed to run this command
		return true
	}

	fmt.Printf("roledetails=%s    user=%s  id=%s\n", roledetails, user.User, userid)

	if roledetails[0] == "discord" {
		// check if users allowed via discord roles
		fmt.Printf("discord role:%s\n", roledetails[1])

		usersDiscordRoles := user.Roles

		for _, v := range usersDiscordRoles {
			fmt.Println("*** iterating over userDiscordRoles")
			//if strconv.Itoa(v.(int)) == roledetails[1] {
			fmt.Printf("v=%s   role=%s\n", v, roledetails[1])
			fmt.Printf("v=%s   role=%s\n", v, strconv.Itoa(viper.GetStringMap("discordroles")[roledetails[1]].(int)))
			if v == strconv.Itoa(viper.GetStringMap("discordroles")[roledetails[1]].(int)) {
				fmt.Println("*** found users discord role")
				return true
			}
		}
		fmt.Println("*** user does not have needed discord role")
		return false

	} else {
		// check normal roles

		fmt.Printf("normal role:%s\n", roledetails[0])

		result := viper.GetStringMap("commandroles")

		for _, users := range result[role].([]interface{}) {
			fmt.Printf(" - %s    type: %T    converted: %T\n", users, users, strconv.Itoa(users.(int)))
		}

		if sliceContainsInt(result[role].([]interface{}), userid) {
			fmt.Printf("Found user %s in %s!\n", userid, role)
			return true
		} else {
			fmt.Println("not here")
		}

		fmt.Println("=================")
	}
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

	roledetails := strings.Split(strings.ToLower(role), ":")

	fmt.Println("roledetails=", roledetails)

	// check if it is a discord role
	if roledetails[0] == "discord" {
		if !viper.IsSet("discordroles") {
			log.Printf("Error: discordroles not configured")
			return false
		}

		if _, ok := viper.GetStringMap("discordroles")[roledetails[1]]; ok {
			fmt.Println("discord roles found")
			return true
		} else {
			fmt.Println("discord roles not found")
		}

		return false
	}

	// check if normal role
	if viper.IsSet("commandroles") {
		fmt.Println("isset commandroles")
		if _, ok := viper.GetStringMap("commandroles")[roledetails[0]]; ok {
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
func sliceContainsInt(i []interface{}, str string) bool {
	fmt.Println("sliceContainsInt start")
	for _, v := range i {
		if strconv.Itoa(v.(int)) == str {
			return true
		}
	}
	return false
}
