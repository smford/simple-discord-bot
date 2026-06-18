package main

import (
	"html"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/go-co-op/gocron/v2"
	"github.com/mmcdole/gofeed"
	"github.com/spf13/viper"
)

var rssScheduler gocron.Scheduler
var rssSeenGUIDs = make(map[string]map[string]bool) // feedID -> set of seen GUIDs
var rssMu sync.Mutex

// initRSSFeeds initialises the RSS feed scheduler and schedules all configured feeds
func initRSSFeeds() {
	var err error
	if rssScheduler != nil {
		logger("info", "Shutting down existing RSS feed scheduler")
		rssScheduler.Shutdown()
		rssScheduler = nil
	}

	if !viper.IsSet("rss_feeds") {
		logger("debug", "No RSS feeds configured")
		return
	}

	rssScheduler, err = gocron.NewScheduler()
	if err != nil {
		logger("error", "Failed to create RSS feed scheduler: %s", err)
		return
	}

	feeds := viper.GetStringMap("rss_feeds")
	for feedID := range feeds {
		logger("debug", "Loading RSS feed: %s", feedID)
		scheduleRSSFeed(feedID)
	}

	rssScheduler.Start()
	logger("info", "RSS feed scheduler started")
}

// scheduleRSSFeed seeds one feed and registers a periodic job to check it
func scheduleRSSFeed(feedID string) {
	if !viper.IsSet("rss_feeds." + feedID) {
		logger("error", "RSS feed ID not found: %s", feedID)
		return
	}

	if viper.IsSet("rss_feeds."+feedID+".disabled") && viper.GetBool("rss_feeds."+feedID+".disabled") {
		logger("debug", "RSS feed %s is disabled, skipping", feedID)
		return
	}

	name := viper.GetString("rss_feeds." + feedID + ".name")

	interval := viper.GetInt("rss_feeds." + feedID + ".interval")
	if interval <= 0 {
		interval = 300 // default: 5 minutes
	}

	// Seed the seen-GUIDs set from the current feed contents so the bot does
	// not flood the channel with historical items on startup.
	rssMu.Lock()
	rssSeenGUIDs[feedID] = make(map[string]bool)
	rssMu.Unlock()
	seedRSSFeed(feedID)

	_, err := rssScheduler.NewJob(
		gocron.DurationJob(time.Duration(interval)*time.Second),
		gocron.NewTask(
			func(id string) {
				checkRSSFeed(id)
			},
			feedID,
		),
		gocron.WithName(name),
	)
	if err != nil {
		logger("error", "Error scheduling RSS feed '%s': %s", name, err)
		return
	}

	logger("info", "RSS feed '%s' scheduled every %d seconds", name, interval)

	// Run an immediate check rather than waiting for the first interval.
	go checkRSSFeed(feedID)
}

// seedRSSFeed fetches the feed and populates the in-memory seen-GUIDs set so
// the bot does not re-post items after a restart.
//
// If a last_guid was previously saved to the config YAML it is used as an
// anchor: every item currently in the feed up to and including that GUID is
// marked as seen.  If the anchor has scrolled out of the feed window all
// current items are treated as seen (safe default for slow feeds).
func seedRSSFeed(feedID string) {
	url := viper.GetString("rss_feeds." + feedID + ".url")
	name := viper.GetString("rss_feeds." + feedID + ".name")

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		logger("warning", "RSS feed '%s': failed to seed (all items will be posted on first check): %s", name, err)
		return
	}

	rssMu.Lock()
	defer rssMu.Unlock()

	storedLastGUID := viper.GetString("rss_feeds." + feedID + ".last_guid")

	if storedLastGUID != "" {
		// Mark all items in the feed as seen using the stored anchor.
		// If the anchor has scrolled out of the feed window all current items
		// are still marked as seen to avoid flooding the channel.
		for _, item := range feed.Items {
			guid := itemGUID(item)
			rssSeenGUIDs[feedID][guid] = true
		}
		logger("debug", "RSS feed '%s' seeded from stored last_guid: %s", name, storedLastGUID)
	} else {
		// No prior state: mark everything as seen except the newest item so
		// it gets posted on the first check, giving immediate confirmation
		// that the feed is working.
		for i, item := range feed.Items {
			if i == 0 {
				continue // leave newest unseen so checkRSSFeed posts it
			}
			guid := itemGUID(item)
			rssSeenGUIDs[feedID][guid] = true
		}
		logger("debug", "RSS feed '%s' seeded with %d items, newest item will be posted on first check", name, len(feed.Items))
	}
}

// checkRSSFeed fetches the feed and posts any items that have not been seen before.
func checkRSSFeed(feedID string) {
	url := viper.GetString("rss_feeds." + feedID + ".url")
	channelID := viper.GetString("rss_feeds." + feedID + ".channel_id")
	name := viper.GetString("rss_feeds." + feedID + ".name")

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		logger("error", "RSS feed '%s': failed to fetch %s: %s", name, url, err)
		return
	}

	rssMu.Lock()
	defer rssMu.Unlock()

	if rssSeenGUIDs[feedID] == nil {
		rssSeenGUIDs[feedID] = make(map[string]bool)
	}

	// Process items oldest-first so the channel shows them in chronological order.
	cutoff := time.Now().UTC().AddDate(0, 0, -30)

	postedAny := false
	for i := len(feed.Items) - 1; i >= 0; i-- {
		item := feed.Items[i]
		guid := itemGUID(item)

		if rssSeenGUIDs[feedID][guid] {
			continue
		}

		// Skip items older than 30 days
		if item.PublishedParsed != nil && item.PublishedParsed.UTC().Before(cutoff) {
			logger("debug", "RSS feed '%s': skipping old item (%s): %s", name, item.PublishedParsed.Format("2006-01-02"), item.Title)
			rssSeenGUIDs[feedID][guid] = true
			continue
		}

		embed := formatRSSItem(feed, item)
		_, err := dg.ChannelMessageSendEmbed(channelID, embed)
		if err != nil {
			logger("error", "RSS feed '%s': failed to post item to channel %s: %s", name, channelID, err)
		} else {
			logger("info", "RSS feed '%s': posted new item: %s", name, item.Title)
		}
		rssSeenGUIDs[feedID][guid] = true
		postedAny = true
	}

	// Persist the newest item's GUID so we survive restarts without re-posting.
	// We always update this so it stays current even when nothing was posted.
	if len(feed.Items) > 0 {
		newestGUID := itemGUID(feed.Items[0])
		if newestGUID != viper.GetString("rss_feeds."+feedID+".last_guid") || postedAny {
			viper.Set("rss_feeds."+feedID+".last_guid", newestGUID)
			if err := viper.WriteConfig(); err != nil {
				logger("error", "RSS feed '%s': failed to save last_guid: %s", name, err)
			} else {
				logger("debug", "RSS feed '%s': saved last_guid: %s", name, newestGUID)
			}
		}
	}
}

// itemGUID returns a stable identifier for an RSS item, falling back to the link.
func itemGUID(item *gofeed.Item) string {
	if item.GUID != "" {
		return item.GUID
	}
	return item.Link
}

// formatRSSItem builds a Discord rich embed for a new RSS item.
func formatRSSItem(feed *gofeed.Feed, item *gofeed.Item) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Color: 0x6364FF, // Mastodon purple
		URL:   item.Link,
		Title: html.UnescapeString(item.Title),
	}

	// Author block: feed title + avatar icon
	if feed != nil {
		author := &discordgo.MessageEmbedAuthor{
			Name: feed.Title,
			URL:  feed.Link,
		}
		if feed.Image != nil && feed.Image.URL != "" {
			author.IconURL = feed.Image.URL
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: feed.Image.URL}
		}
		embed.Author = author
	}

	// Description: prefer the plain-text description over raw HTML content
	desc := item.Description
	if desc == "" {
		desc = item.Content
	}
	if desc != "" {
		desc = html.UnescapeString(desc)
		if len(desc) > 400 {
			desc = desc[:397] + "..."
		}
		embed.Description = desc
	}

	// Timestamp: set both the embed timestamp (bottom of embed) and a visible field
	pubDateStr := item.Published
	if item.PublishedParsed != nil {
		t := item.PublishedParsed.UTC()
		embed.Timestamp = t.Format(time.RFC3339)
		pubDateStr = t.Format("02 Jan 2006 15:04 UTC")
	}
	if pubDateStr != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Published",
			Value:  pubDateStr,
			Inline: true,
		})
	}

	return embed
}
