package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/mmcdole/gofeed"
)

const DOMAIN = "nyaa.si"

func latestIndexedPost(db *sql.DB) (id int, err error) {
	err = db.QueryRow("SELECT id FROM posts ORDER BY id DESC LIMIT 1").Scan(&id)

	return
}

func newestPost() (id int, err error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://" + DOMAIN + "/?page=rss")
	if err != nil {
		return 0, err
	}

	rawID, _ := strings.CutPrefix(feed.Items[0].Link, "https://"+DOMAIN+"/download/")
	rawID, _ = strings.CutSuffix(rawID, ".torrent")
	id, err = strconv.Atoi(rawID)
	return
}

const DefaultStartID = 0
const DefaultDatabase = "file:database.db?cache=shared"

type Config struct {
	StartID      int
	EndID        int
	Database     string
	SkipExisting bool
}

func main() {
	newestPostID, err := newestPost()
	if err != nil {
		log.Panic(err)
	}

	var config Config
	flag.IntVar(&config.StartID, "start", DefaultStartID, "ID to start scraping on (default: biggest ID in database + 1")
	flag.IntVar(&config.EndID, "end", newestPostID, "ID to end scraping on")
	flag.StringVar(&config.Database, "database", DefaultDatabase, "sqlite database path")
	flag.BoolVar(&config.SkipExisting, "skip", false, "skips existing post entries")
	flag.Parse()

	signalChan := make(chan os.Signal, 1)

	signal.Notify(
		signalChan,
		syscall.SIGHUP,  // kill -SIGHUP XXXX
		syscall.SIGINT,  // kill -SIGINT XXXX or Ctrl+c
		syscall.SIGQUIT, // kill -SIGQUIT XXXX
	)

	db, err := initializeApplication(&config)
	defer db.Close()
	if err != nil {
		log.Panic(err)
	}

	if config.StartID == DefaultStartID {
		config.StartID, err = latestIndexedPost(db)
		if err != nil {
			config.StartID = DefaultStartID
		}
	}

	pageInfoFinder := newPageInfoFinder()

	var (
		info        *PageInfo
		status      int
		currentTime time.Time = time.Now()
		shouldExit            = false
	)

	go func() {
		<-signalChan
		shouldExit = true
	}()

	for currentID := config.StartID + 1; currentID <= newestPostID && !shouldExit; currentID++ {
		if config.SkipExisting {
			var tempID int
			if err = db.QueryRow("SELECT id FROM posts WHERE id=?", currentID).Scan(&tempID); err == nil {
				fmt.Println("Skipping", tempID)
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				log.Fatal(err)
			}
		}

		// Try until parsing works
		for {
			if status, info, err = pageInfoFinder.getBasicInfo(currentID); err == nil {
				break
			}

			log.Println("Warning:", err)
			RandomSleep(&currentTime)
		}

		statusUpdate := fmt.Sprintf("[ID %d] %d %v\n", currentID, status, time.Since(currentTime))
		if status == http.StatusOK {
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				log.Fatal(err)
			}

			if _, err = tx.Exec("INSERT OR IGNORE INTO posts (id, deleted, category, submitter, information, file_size, date, seeders, leechers, torrent_url, magnet_url, completed, info_hash, description) VALUES (?, false, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", currentID, info.Category, info.Submitter, info.Information, info.FileSize, info.Date, info.Seeders, info.Leechers, info.TorrentUrl, info.MagnetUrl, info.Completed, info.InfoHash, info.Description); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			if _, err = tx.Exec(PostUpdateLastFetchedQuery, currentID); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			for _, comment := range info.Comments {
				// updates comment info
				var editedDate any
				if err = tx.QueryRow("SELECT edited_date FROM comments WHERE id=?", comment.ID).Scan(&editedDate); errors.Is(err, sql.ErrNoRows) {
					if _, err = tx.Exec("INSERT INTO comments (id, submitter, content, date, post_id, deleted, edited_date, last_fetched) VALUES (?, ?, ?, ?, ?, false, ?, datetime('now'))", comment.ID, comment.Submitter, comment.Content, comment.Date, currentID, comment.EditedDate); err != nil {
						tx.Rollback()
						log.Fatal(err)
					}
				} else if err != nil {
					tx.Rollback()
					log.Fatal(err)
				} else if editedDate != comment.EditedDate {
					if _, err = tx.Exec("UPDATE comments SET content=?, edited_date=?, last_fetched=datetime('now') WHERE id=?", comment.Content, comment.EditedDate, comment.ID); err != nil {
						tx.Rollback()
						log.Fatal(err)
					}
				}
			}

			rows, err := tx.Query("SELECT id, submitter, content, date, edited_date FROM comments WHERE post_id=?", currentID)
			if err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			var comment Comment
			for rows.Next() {
				if err = rows.Scan(&comment.ID, &comment.Submitter, &comment.Content, &comment.Date, &comment.EditedDate); err != nil {
					tx.Rollback()
					log.Fatal(err)
				}

				if !Contains(info.Comments, comment) {
					if _, err = tx.Exec("UPDATE comments SET deleted=true, last_fetched=datetime('now') WHERE id=?", comment.ID); err != nil {
						tx.Rollback()
						log.Fatal(err)
					}
				}
			}

			if err = tx.Commit(); err != nil {
				log.Fatal(err)
			}

			color.Green(statusUpdate)
		} else if status == http.StatusNotFound {
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				log.Fatal(err)
			}

			if _, err = tx.Exec("INSERT OR IGNORE INTO posts (id, deleted) VALUES (?, true)", currentID); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			if _, err = tx.Exec(PostUpdateLastFetchedQuery, currentID); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			if err = tx.Commit(); err != nil {
				log.Fatal(err)
			}

			color.Red(statusUpdate)
		} else {
			log.Fatal("Status:", status)
		}

		RandomSleep(&currentTime)
	}

	if shouldExit {
		fmt.Println("Exiting early")
	} else {
		fmt.Println("Finished going through all ids :)")
	}
}

func RandomSleep(date *time.Time) {
	sleepTime := 300 + rand.Intn(500)
	fmt.Println("💤", sleepTime, "ms")
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	*date = time.Now()
}

func Contains[T comparable](array []T, value T) bool {
	for _, j := range array {
		if j == value {
			return true
		}
	}

	return false
}

var PostUpdateLastFetchedQuery string = "UPDATE posts SET last_fetched=datetime('now') WHERE id=?"
