package main

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

var taskScheduler gocron.Scheduler
var jobs map[string]gocron.Job = make(map[string]gocron.Job)

// Initializes the task scheduler
func initTasks() {
	var err error
	if taskScheduler != nil {
		logger("info", "Shutting down existing Tasks scheduler")
		taskScheduler.Shutdown()
		taskScheduler = nil
	}
	if taskScheduler == nil {
		logger("info", "Creating Tasks scheduler")

		taskScheduler, err = gocron.NewScheduler()
		if err != nil {
			logger("error", "Failed to create new Tasks scheduler: %s", err)
			return
		}
	}

	// load the tasks from the config
	tasks := viper.GetStringMap("tasks.tasks")
	for taskID, _ := range tasks {
		logger("debug", "Loading task ID: %s", taskID)
		scheduleTask(taskID)
	}

	// start the scheduler
	taskScheduler.Start()
	logger("info", "Task scheduler started")
}

// Schedules a task
func scheduleTask(taskID string) {

	logger("debug", "Scheduling task %s", taskID)
	if !viper.IsSet("tasks.tasks." + taskID) {
		logger("error", "Task ID not found: %s", taskID)
		return
	}

	if viper.IsSet("tasks.tasks." + taskID + ".archived") {
		if viper.GetBool("tasks.tasks." + taskID + ".archived") {
			logger("debug", "Task ID %s is archived, not scheduling", taskID)
			return
		}
	}

	schedule := viper.GetString("tasks.tasks." + taskID + ".schedule")
	name := viper.GetString("tasks.tasks." + taskID + ".name")

	job, err := taskScheduler.NewJob(
		gocron.CronJob(
			schedule,
			false,
		),
		gocron.NewTask(
			func(a string) {
				sendTaskMessage(dg, a)
			},
			taskID,
		),
		gocron.WithName(name),
		gocron.WithIdentifier(uuid.MustParse(taskID)),
	)
	if err != nil {
		logger("error", "Error scheduling task - Task ID: %s - Error: %s", taskID, err)
		return
	}

	jobs[taskID] = job

	logger("info", "Task '%s' scheduled - ID: %s Cron: %s", name, taskID, schedule)
}

// Adds a new task
func taskAdd(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	parts := splitQuotedParts(content)

	if len(parts) < 3 {
		privateMessageCreate(s, m.Author.ID, "Invalid command format", false)
		return
	}

	newTaskID := uuid.New().String()
	newTask := "tasks.tasks." + newTaskID

	name := parts[0]
	cronSchedule := parts[1]
	description := parts[2]

	viper.Set(newTask+".schedule", cronSchedule)
	viper.Set(newTask+".name", name)
	viper.Set(newTask+".description", description)
	viper.WriteConfig()

	scheduleTask(newTaskID)

	privateMessageCreate(s, m.Author.ID, "Task added", false)
	logger("info", "Task added")
}

// Archives a task
func taskArchive(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	id := content

	if !viper.IsSet("tasks.tasks." + id) {
		privateMessageCreate(s, m.Author.ID, "Task item not found", false)
		return
	}

	taskUUID, err := uuid.Parse(id)
	if err != nil {
		logger("error", "Invalid task scheduler ID: %s", id)
		privateMessageCreate(s, m.Author.ID, "Failed to remove task from scheduler due to invalid ID", false)
		return
	}
	taskScheduler.RemoveJob(taskUUID)

	// Get all tasks to preserve all data
	allTasks := viper.GetStringMap("tasks.tasks")

	// Preserve all tasks and their fields
	for taskID, taskData := range allTasks {
		if taskMap, ok := taskData.(map[string]interface{}); ok {
			for key, value := range taskMap {
				viper.Set("tasks.tasks."+taskID+"."+key, value)
			}
		}
	}
	viper.Set("tasks.tasks."+id+".archived", true)
	viper.WriteConfig()

	logger("warning", "Task item archived")
	privateMessageCreate(s, m.Author.ID, "Task item archived", false)
}

// Runs a scheduled task immediately
func taskRun(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	if content == "" {
		privateMessageCreate(s, m.Author.ID, "No task ID provided", false)
		return
	}

	taskID := content

	if !viper.IsSet("tasks.tasks." + taskID) {
		privateMessageCreate(s, m.Author.ID, "Task ID not found", false)
		return
	}

}

// Lists scheduled tasks
func taskListScheduled(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	if taskScheduler == nil {
		logger("error", "Task scheduler is not initialised")
		privateMessageCreate(s, m.Author.ID, "Task scheduler is not initialised", false)
		return
	}
	jobs := taskScheduler.Jobs()
	if len(jobs) == 0 {
		logger("debug", "No scheduled tasks found")
		privateMessageCreate(s, m.Author.ID, "No scheduled tasks found", false)
		return
	}
	logger("info", "Scheduled tasks found")
	var jobItems string
	for _, job := range jobs {
		jobID := job.ID()
		jobName := job.Name()
		jobNextRun, _ := job.NextRun()
		jobLastRun, lastRunExists := job.LastRun()

		if lastRunExists != nil || jobLastRun.IsZero() {
			jobItems += fmt.Sprintf(
				"## Name: %s\n**ID:** %s\n**Next Run:** %s\n\n",
				jobName,
				jobID,
				jobNextRun.In(timezone).Format("2006-01-02")+" "+jobNextRun.In(timezone).Format("15:04:05"),
			)
		} else {
			jobItems += fmt.Sprintf(
				"## Name: %s\n**ID:** %s\n**Next Run:** %s\n**Last Run:** %s\n\n",
				jobName,
				jobID,
				jobNextRun.In(timezone).Format("2006-01-02")+" "+jobNextRun.In(timezone).Format("15:04:05"),
				jobLastRun.In(timezone).Format("2006-01-02")+" "+jobLastRun.In(timezone).Format("15:04:05"),
			)
		}
	}
	if jobItems == "" {
		privateMessageCreate(s, m.Author.ID, "No scheduled tasks found", false)
		return
	}
	privateMessageCreate(s, m.Author.ID, "# Scheduler for Tasks\n"+jobItems, false)
	logger("debug", "Scheduled tasks sent")
}

// Lists tasks
func taskList(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	if !viper.IsSet("tasks.tasks") {
		privateMessageCreate(s, m.Author.ID, "No tasks found", false)
		return
	}

	taskList := viper.GetStringMap("tasks.tasks")

	if len(taskList) == 0 {
		privateMessageCreate(s, m.Author.ID, "No tasks found", false)
		return
	}

	var taskItems string
	for taskID, _ := range taskList {
		schedule := viper.Get("tasks.tasks." + taskID + ".schedule")
		name := viper.Get("tasks.tasks." + taskID + ".name")
		description := viper.Get("tasks.tasks." + taskID + ".description")
		archived := viper.Get("tasks.tasks." + taskID + ".archived")
		if archived == nil || !archived.(bool) {
			taskItems += fmt.Sprintf("## Name: %v\n**ID:** %s\n**Schedule:** `%v`\n**Description:** %v\n\n", name, taskID, schedule, description)
		}
	}
	if taskItems == "" {
		privateMessageCreate(s, m.Author.ID, "No tasks found", false)
		return
	}
	privateMessageCreate(s, m.Author.ID, "# Tasks\n"+taskItems, false)
	logger("debug", "Task list sent")
}

// Sends a task message to the designated channel
func sendTaskMessage(s *discordgo.Session, taskID string) {

	taskChannelID := viper.GetString("tasks.channel_id")

	taskTitle := viper.GetString("tasks.tasks." + taskID + ".name")
	taskDescription := viper.GetString("tasks.tasks." + taskID + ".description")

	components := []discordgo.MessageComponent{}

	actionRow := discordgo.ActionsRow{}
	actionRow = discordgo.ActionsRow{Components: []discordgo.MessageComponent{}}

	actionButton := discordgo.Button{
		Label:    "Completed",
		Style:    discordgo.SuccessButton,
		CustomID: "Task:" + taskID + ":Completed",
		Disabled: false,
	}
	actionRow.Components = append(actionRow.Components, actionButton)
	components = append(components, actionRow)

	logger("info", "Creating task thread")

	outstandingTagID := viper.GetString("tasks.outstanding_tag_id")

	threadData := discordgo.ThreadStart{
		Name:                taskTitle,
		AutoArchiveDuration: 60 * 24 * 7,
		Type:                13,
		AppliedTags:         []string{outstandingTagID},
	}

	message := discordgo.MessageSend{
		Content:    taskDescription,
		Components: components,
	}

	_, err := s.ForumThreadStartComplex(taskChannelID, &threadData, &message)
	if err != nil {
		logger("error", "Error creating task message: %s", err)
	}

	logger("info", "Created task thread")

}

// Handles task interactions
func taskInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:

		interactionParameters, _ := splitStringArray(i.MessageComponentData().CustomID, ":", 3)

		if interactionParameters[0] != "Task" {
			return
		}

		logger("info", "Task interaction handler")
		logger("debug", "Task interaction handler: %s", i.MessageComponentData().CustomID)
		logger("debug", "Task interaction handler: %s", i.Member.User.Username)

		outstandingTagID := viper.GetString("tasks.outstanding_tag_id")
		completedTagID := viper.GetString("tasks.completed_tag_id")

		removeTagFromThread(dg, i.ChannelID, outstandingTagID)
		addTagToThread(dg, i.ChannelID, completedTagID)

		sendMessageToDiscord(i.ChannelID, "Task completed by **"+getDiscordDisplayName(i.Member)+"** on **"+time.Now().In(timezone).Format("2006-01-02")+"** at **"+time.Now().In(timezone).Format("15:04:05")+"**")

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Task marked as completed",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			logger("error", "Error sending task complete response message to user: %s", err)
		}

		threadArchived := true
		_, err = s.ChannelEdit(i.ChannelID, &discordgo.ChannelEdit{
			Archived: &threadArchived,
			Locked:   &threadArchived,
		})
		if err != nil {
			logger("error", "Could not close thread ThreadID: %s - Error: %s", i.ChannelID, err)
		}

	}
}
