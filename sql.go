package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"log"
)

func initializeApplication(config *Config) (db *sql.DB, err error) {
	db, err = sql.Open("sqlite3", config.Database)
	if err != nil {
		log.Fatal(err)
	}

	sqlStmt := `
	create table if not exists posts (
	    id integer not null primary key,
	    deleted boolean,
		title text,
	    category text,
	    submitter text,
	    information text,
	    file_size int,
	    date datetime,
	    seeders int,
	    leechers int,
	    completed int,
	    info_hash text,
	    description text,
	    torrent_url text,
	    magnet_url text,
	    last_fetched datetime
	);
	create table if not exists comments (
		id integer not null primary key,
		deleted boolean,
		submitter text,
		content text,
		date datetime,
		edited_date datetime,
		post_id integer not null,
		last_fetched datetime,

		FOREIGN KEY (post_id) REFERENCES posts (id)
	);
	create table if not exists folders (
	    id integer not null primary key,
	    folder_name text,
	    post_id integer not null,

		FOREIGN KEY (post_id) REFERENCES posts (id)
	);
	create table if not exists files (
	    file_name text not null,
	    file_size integer not null,
	    folder_id integer not null,
	    post_id integer not null,

	    FOREIGN KEY (folder_id) REFERENCES folders (id),
		FOREIGN KEY (post_id) REFERENCES posts (id)
	);
	`

	_, err = db.Exec(sqlStmt)

	return
}

func latestIndexedPost(db *sql.DB) (id int, err error) {
	err = db.QueryRow("SELECT id FROM posts ORDER BY id DESC LIMIT 1").Scan(&id)

	return
}
