package main

import (
	"fmt"
	"github.com/mmcdole/gofeed"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

func RandomSleep(date *time.Time) {
	sleepTime := 300 + rand.Intn(400)
	fmt.Println("💤", sleepTime, "ms")
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	*date = time.Now()
}

func CommentsContains(comments []Comment, ID int) bool {
	for _, comment := range comments {
		if comment.ID == ID {
			return true
		}
	}

	return false
}

// Finds the newest torrent by reading the rss feed
func NewestPost() (id int, err error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://" + Domain + "/?page=rss")
	if err != nil {
		return 0, err
	}

	rawID, _ := strings.CutPrefix(feed.Items[0].Link, "https://"+Domain+"/download/")
	rawID, _ = strings.CutSuffix(rawID, ".torrent")
	id, err = strconv.Atoi(rawID)
	return
}
