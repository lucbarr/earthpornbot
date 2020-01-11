package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/jzelinskie/geddit"
	"github.com/spf13/viper"
)

// Config is the configuration to access the reddit api
type Config struct {
	User         string
	Password     string
	ClientID     string
	ClientSecret string

	Limit int32

	MaxConcurrentRoutines int32
}

func defaultConfig() *Config {
	return &Config{
		User:                  viper.GetString("credentials.user"),
		Password:              viper.GetString("credentials.password"),
		ClientID:              viper.GetString("credentials.app.client-id"),
		ClientSecret:          viper.GetString("credentials.app.client-secret"),
		Limit:                 viper.GetInt32("subreddit.submissions.limit"),
		MaxConcurrentRoutines: viper.GetInt32("app.maxConcurrentRoutines"),
	}
}

// Reddit is used to get the reddit images
type Reddit struct {
	cfg       *Config
	subreddit string
	limit     int32

	session           *geddit.OAuthSession
	client            *http.Client
	allowedExtMatches []*regexp.Regexp
}

// NewReddit creates a structure to access Reddit API
func NewReddit() *Reddit {

	allowedExt := viper.GetStringSlice("subreddit.submissions.allowedExtensions")
	allowedExtMatches := make([]*regexp.Regexp, 0, len(allowedExt))
	for _, ext := range allowedExt {
		pattern := fmt.Sprintf("^.+\\.%s$", ext)
		allowedExtMatches = append(allowedExtMatches, regexp.MustCompile(pattern))
	}

	return &Reddit{
		cfg:               defaultConfig(),
		subreddit:         viper.GetString("subreddit.name"),
		client:            &http.Client{},
		allowedExtMatches: allowedExtMatches,
	}
}

// Authenticate authenticates the api
func (r *Reddit) Authenticate() error {
	o, err := geddit.NewOAuthSession(
		r.cfg.ClientID,
		r.cfg.ClientSecret,
		"bot for r/earthporn by u/earthpornsuperbot",
		"",
	)
	if err != nil {
		return err
	}

	err = o.LoginAuth(r.cfg.User, r.cfg.Password)
	if err != nil {
		return err
	}

	r.session = o
	return nil
}

// FetchSubmissions fetches submissions
func (r *Reddit) FetchSubmissions() error {
	validURLs := r.fetchSubmissions()

	fetchImage := func(url string, done chan bool, abort chan error) {
		regx := regexp.MustCompile("[^/]*$")
		matches := regx.FindAllString(url, 1)
		if len(matches) == 0 {
			done <- false
			abort <- fmt.Errorf("No match for regex")
			return
		}

		filename := matches[0]
		file, err := os.Create(filename)
		if err != nil {
			done <- false
			abort <- fmt.Errorf("Could not create file %s", filename)
			return
		}
		defer file.Close()

		resp, err := r.client.Head(url)
		if err != nil {
			done <- false
			abort <- fmt.Errorf("could not get HEAD")
			return
		}
		fmt.Printf("Getting image %s, length: %s\n", url, resp.Header.Get("Content-Length"))
		resp, err = r.client.Get(url)
		if err != nil {
			done <- false
			abort <- fmt.Errorf("No match for regex")
			return
		}
		defer resp.Body.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			done <- false
			abort <- fmt.Errorf("No match for regex")
			return
		}
		done <- true
	}

	done := make(chan bool)
	abort := make(chan error)
	for _, url := range validURLs {
		go fetchImage(url, done, abort)
	}

	for i := 0; i < len(validURLs); i++ {
		if (<-done) == false {
			return <-abort
		}
	}
	return nil
}

func (r *Reddit) fetchSubmissions() []string {
	opts := geddit.ListingOptions{
		Limit: int(r.cfg.Limit),
	}

	posts, err := r.session.SubredditSubmissions("earthporn", geddit.HotSubmissions, opts)
	if err != nil {
		log.Fatal(err)
	}

	isImageURL := func(s string) bool {
		ret := false
		for _, regex := range r.allowedExtMatches {
			ret = ret || regex.MatchString(s)
		}
		return ret
	}

	validURLs := []string{}
	for _, p := range posts {
		if isImageURL(p.URL) {
			validURLs = append(validURLs, p.URL)
		}
	}
	return validURLs
}
