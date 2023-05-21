package main

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"

	"astuart.co/goq"
	"github.com/dustin/go-humanize"
	"github.com/valyala/fasthttp"
)

type PageInfoFinder struct {
	req    fasthttp.Request
	resp   fasthttp.Response
	url    fasthttp.URI
	client fasthttp.Client
}

func newPageInfoFinder() (p PageInfoFinder) {
	p.req.Header.SetMethod(fasthttp.MethodGet)
	p.url.SetScheme("https")
	p.url.SetHost(DOMAIN)

	return
}

func (c *PageInfoFinder) getBasicInfo(id int) (status int, info *PageInfo, err error) {
	c.url.SetPath("/view/" + strconv.Itoa(id))
	c.req.SetURI(&c.url)
	if err = c.client.Do(&c.req, &c.resp); err != nil {
		return
	}

	status = c.resp.StatusCode()

	if status != http.StatusOK {
		return
	}

	b := c.resp.Body()
	var rawInfo RawPageInfo
	if err = goq.NewDecoder(bytes.NewReader(b)).Decode(&rawInfo); err != nil {
		return
	}

	// TODO: filelist parsing
	info, err = rawInfo.Process()
	return
}

type PageInfo struct {
	Title       string
	Category    string
	Submitter   string
	Information string
	FileSize    uint64
	Date        time.Time
	Seeders     int
	Leechers    int
	Completed   int
	InfoHash    string
	Description string
	TorrentUrl  string
	MagnetUrl   string
	Comments    []Comment
}

type RawPageInfo struct {
	Title       string       `goquery:".panel:nth-child(1) .panel-heading .panel-title:nth-child(1)"`
	RawCategory string       `goquery:".col-md-5 a:nth-child(2),[href]"`
	Submitter   string       `goquery:".col-md-5 a.text-default[href]"`
	Information string       `goquery:".row:nth-child(3) .col-md-5:nth-child(2)"`
	RawFileSize string       `goquery:".row:nth-child(4) .col-md-5:nth-child(2)"`
	RawDate     string       `goquery:".col-md-5[data-timestamp]"`
	Seeders     int          `goquery:".row:nth-child(2) .col-md-5 span"`
	Leechers    int          `goquery:".row:nth-child(3) .col-md-5 span"`
	Completed   int          `goquery:".row:nth-child(4) .col-md-5:nth-child(4)"`
	InfoHash    string       `goquery:"kbd"`
	Description string       `goquery:"#torrent-description"`
	TorrentUrl  string       `goquery:".panel-footer a:nth-child(1),[href]"`
	MagnetUrl   string       `goquery:".card-footer-item,[href]"`
	RawComments []RawComment `goquery:".comment-panel"`
}

func (info *RawPageInfo) Process() (*PageInfo, error) {
	var Comments []Comment
	for _, rawComment := range info.RawComments {
		c, err := rawComment.Process()
		if err != nil {
			return nil, err
		}

		Comments = append(Comments, *c)
	}

	Date, err := time.Parse("2006-01-02 15:04 UTC", info.RawDate)
	if err != nil {
		return nil, err
	}

	Category, _ := strings.CutPrefix(info.RawCategory, "/?c=")

	// humanize can't handle sizes such as "0 Bytes", so it has to be parsed manually
	var FileSize uint64
	if withoutSuffix, found := strings.CutSuffix(info.RawFileSize, " Bytes"); found {
		FileSize, err = strconv.ParseUint(withoutSuffix, 10, 64)
	} else {
		FileSize, err = humanize.ParseBytes(info.RawFileSize)
	}
	if err != nil {
		return nil, err
	}

	return &PageInfo{
		Title:       info.Title,
		Category:    Category,
		Submitter:   info.Submitter,
		Information: info.Information,
		FileSize:    FileSize,
		Date:        Date,
		Seeders:     info.Seeders,
		Leechers:    info.Leechers,
		Completed:   info.Completed,
		InfoHash:    info.InfoHash,
		Description: info.Description,
		TorrentUrl:  info.TorrentUrl,
		MagnetUrl:   info.MagnetUrl,
		Comments:    Comments,
	}, nil
}

type Comment struct {
	ID         int
	Submitter  string
	Content    string
	Date       time.Time
	EditedDate any // time.Time or nil
}

type RawComment struct {
	RawID         string `goquery:".comment-content,[id]"` // torrent-comment0000
	Submitter     string `goquery:"[title=\"User\"]"`
	Content       string `goquery:".comment-content"`
	RawDate       string `goquery:".comment-details a small[data-timestamp]"` // 2022-09-15 16:43 UTC
	RawEditedDate string `goquery:".comment-details > small,[title]"`         // 2021-07-01 18:51:45
}

func (r *RawComment) Process() (*Comment, error) {
	RawID, _ := strings.CutPrefix(r.RawID, "torrent-comment")
	ID, err := strconv.Atoi(RawID)
	if err != nil {
		return nil, err
	}

	Date, err := time.Parse("2006-01-02 15:04 UTC", r.RawDate)
	if err != nil {
		return nil, err
	}

	var EditedDate any
	if r.RawEditedDate != "" {
		EditedDate, err = time.Parse("2006-01-02 15:04:05", r.RawEditedDate)
		if err != nil {
			return nil, err
		}
	}

	return &Comment{
		Submitter:  r.Submitter,
		Content:    r.Content,
		Date:       Date,
		EditedDate: EditedDate,
		ID:         ID,
	}, nil
}
