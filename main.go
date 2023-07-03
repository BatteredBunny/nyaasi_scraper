package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
)

type Config struct {
	StartID         int
	EndID           int
	Database        string
	SkipExisting    bool
	ContinueInRange bool
}

var UpdateCommentQuery = "UPDATE comments SET content=?, edited_date=?, last_fetched=datetime('now') WHERE id=?"
var FindCommentEditDate = "SELECT edited_date FROM comments WHERE id=?"
var InsertNewComment = "INSERT INTO comments (id, submitter, content, date, post_id, deleted, edited_date, last_fetched) VALUES (?, ?, ?, ?, ?, false, ?, datetime('now'))"
var GetCommentByPostID = "SELECT id FROM comments WHERE post_id=?"
var SetCommentAsDeleted = "UPDATE comments SET deleted=true, last_fetched=datetime('now') WHERE id=?"

var InsertNewPost = "INSERT OR IGNORE INTO posts (id, deleted, title, category, submitter, information, file_size, date, seeders, leechers, torrent_url, magnet_url, completed, info_hash, description) VALUES (?, false, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
var UpdatePostLastFetched = "UPDATE posts SET last_fetched=datetime('now') WHERE id=?"
var GetPostID = "SELECT id FROM posts WHERE id=?"
var InsertDeletedPost = "INSERT OR IGNORE INTO posts (id, deleted) VALUES (?, true)"

var rangeSkipQuery = "SELECT id FROM posts WHERE id BETWEEN ? AND ? ORDER BY id DESC LIMIT 1"

func main() {
	newestPostID, err := newestPost()
	if err != nil {
		log.Panic(err)
	}

	var config Config
	flag.IntVar(&config.StartID, "start", DefaultStartID, "ID to start scraping on (default: biggest ID in database + 1")
	flag.IntVar(&config.EndID, "end", newestPostID, "ID to end scraping on")
	flag.StringVar(&config.Database, "database", DefaultDatabaseConnectionString, "sqlite database path")
	flag.BoolVar(&config.SkipExisting, "skip", false, "skips existing post entries")
	flag.BoolVar(&config.ContinueInRange, "continue-in-range", false, "skips large chunks of existing entries")
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
		currentTime = time.Now()
		shouldExit  = false
	)

	go func() {
		<-signalChan
		shouldExit = true
	}()

	fmt.Println("Starting from", config.StartID+1, "and ending on:", config.EndID)

	// basically latest indexed post but in the range specified in flags
	var rangeSkip int
	if config.ContinueInRange {
		if err = db.QueryRow(rangeSkipQuery,
			config.StartID,
			config.EndID,
		).Scan(&rangeSkip); err == nil {
			rangeSkip += 1
			fmt.Println("Skipping in range", rangeSkip)
		}
	}

	for currentID := config.StartID + 1; currentID <= config.EndID && !shouldExit; currentID++ {
		if config.ContinueInRange {
			if currentID <= rangeSkip {
				continue
			}
		}

		if config.SkipExisting {
			var tempID int
			if err = db.QueryRow(GetPostID, currentID).Scan(&tempID); err == nil {
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
			var tx *sql.Tx
			tx, err = db.BeginTx(context.Background(), nil)
			if err != nil {
				log.Fatal(err)
			}

			if _, err = tx.Exec(InsertNewPost,
				currentID,
				info.Title,
				info.Category,
				info.Submitter,
				info.Information,
				info.FileSize,
				info.Date,
				info.Seeders,
				info.Leechers,
				info.TorrentUrl,
				info.MagnetUrl,
				info.Completed,
				info.InfoHash,
				info.Description,
			); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			if _, err = tx.Exec(UpdatePostLastFetched, currentID); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			// updates comment info
			var editedDate any
			for _, comment := range info.Comments {
				if err = tx.QueryRow(FindCommentEditDate, comment.ID).Scan(&editedDate); errors.Is(err, sql.ErrNoRows) {
					if _, err = tx.Exec(InsertNewComment,
						comment.ID,
						comment.Submitter,
						comment.Content,
						comment.Date,
						currentID,
						comment.EditedDate,
					); err != nil {
						tx.Rollback()
						log.Fatal(err)
					}
				} else if err != nil {
					tx.Rollback()
					log.Fatal(err)
				} else if editedDate != comment.EditedDate {
					if _, err = tx.Exec(UpdateCommentQuery,
						comment.Content,
						comment.EditedDate,
						comment.ID,
					); err != nil {
						tx.Rollback()
						log.Fatal(err)
					}
				}
			}

			var rows *sql.Rows
			rows, err = tx.Query(GetCommentByPostID, currentID)
			if err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			// marks missing comments as deleted
			var commentID int
			for rows.Next() {
				if err = rows.Scan(&commentID); err != nil {
					tx.Rollback()
					log.Fatal(err)
				}

				if !CommentsContains(info.Comments, commentID) {
					if _, err = tx.Exec(SetCommentAsDeleted, commentID); err != nil {
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
			var tx *sql.Tx
			tx, err = db.BeginTx(context.Background(), nil)
			if err != nil {
				log.Fatal(err)
			}

			if _, err = tx.Exec(InsertDeletedPost, currentID); err != nil {
				tx.Rollback()
				log.Fatal(err)
			}

			if _, err = tx.Exec(UpdatePostLastFetched, currentID); err != nil {
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
