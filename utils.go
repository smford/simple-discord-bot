package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

func prepareTemplate(message string, commandoptions map[string]string) string {
	// do all the templating, replace {0} etc in the command with the options the user has given
	for key, value := range commandoptions {
		message = strings.Replace(message, key, value, -1)
	}

	return message
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

// make a query to a url
func downloadApi(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		logger("error", "Could not connect to api url: \"%s\" with error: %s", url, err)
		return "error"
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			logger("error", "Error with API request URL \"%s\", Message: %s", url, err)
			return "Could not make API request"
		}

		return string(body)
	} else {
		logger("error", "Could not make API request "+url+" HTTPStatus: "+strconv.Itoa(resp.StatusCode))
		return "Could not make API request"
	}
}

// runs a shell command and gathers output
func shellOut(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command(viper.GetString("_shell"), "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// splits (chunks) a message
func chunkMessage(message string, max int) map[int]string {
	sS := 0
	finished := false
	n := 0
	delimchar := "\n"
	messagemap := make(map[int]string)

	for !finished {
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
				if v == strconv.Itoa(viper.GetStringMap("discord_roles")[roledetails[1]].(int)) {
					// found users discord role
					return true
				}
			}
		}

		// user does not have needed discord role
		return false

	} else {
		// check normal roles

		result := viper.GetStringMap("discord_roles_users")

		if sliceContainsInt(result[role].([]interface{}), userid) {
			// user has a role
			return true
		}

	}
	return false
}

// list normal roles and the users
func listRoles() {
	for k, v := range viper.GetStringMap("discord_roles_users") {
		fmt.Printf("Role: %s\n", k)
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
		if !viper.IsSet("discord_roles") {
			logger("error", "Configuration variable 'discord_roles' not configured")
			return false
		}

		if _, ok := viper.GetStringMap("discord_roles")[roledetails[1]]; ok {
			// found valid discord role
			return true
		}

		// no valid discord role found
		return false
	}

	// check if normal role
	if viper.IsSet("discord_roles_users") {
		if _, ok := viper.GetStringMap("discord_roles_users")[roledetails[0]]; ok {
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

// Helper function to check if a slice contains a value
func sliceContainsValue(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// Helper function to check if a string is empty and return a default value
func isEmptyOrDefault(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// Helper function to compare two arrays and return the added, removed, and existing elements
func diffArrays[T comparable](oldArray []T, newArray []T) (added []T, removed []T, existing []T) {
	oldMap := make(map[T]bool)
	newMap := make(map[T]bool)

	for _, v := range oldArray {
		oldMap[v] = true
	}

	for _, v := range newArray {
		newMap[v] = true
		if !oldMap[v] {
			added = append(added, v)
		} else {
			existing = append(existing, v)
		}
	}

	for _, v := range oldArray {
		if !newMap[v] {
			removed = append(removed, v)
		}
	}

	return added, removed, existing
}

// splitString splits a string into the first wordCount words and the remaining words.
func splitString(input string, splitCharacter string, wordCount int) (string, string) {
	words := strings.Split(input, splitCharacter) // Split the string into words
	if len(words) <= wordCount {
		return strings.Join(words, splitCharacter), "" // If wordCount or fewer words, return all as the first part
	}
	return strings.Join(words[:wordCount], splitCharacter), strings.Join(words[wordCount:], splitCharacter) // Split into first wordCount and the rest
}

// splitStringArray splits a string into the first wordCount words and the remaining words as an array.
func splitStringArray(input string, splitCharacter string, wordCount int) ([]string, []string) {
	words := strings.Split(input, splitCharacter) // Split the string into words
	if len(words) <= wordCount {
		return words, nil // If wordCount or fewer words, return all as a single element
	}
	return words[:wordCount], words[wordCount:]

}

// splitQuotedParts splits a string into parts enclosed in quotes.
func splitQuotedParts(input string) []string {
	// Regular expression to match quoted parts
	re := regexp.MustCompile(`"([^"]*)"`)
	matches := re.FindAllStringSubmatch(input, -1)

	var parts []string
	for _, match := range matches {
		if len(match) > 1 {
			parts = append(parts, match[1]) // Add the captured group (without quotes)
		}
	}
	return parts
}
