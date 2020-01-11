package api

import (
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

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
}

func defaultConfig() *Config {
	return &Config{
		User:         viper.GetString("credentials.user"),
		Password:     viper.GetString("credentials.password"),
		ClientID:     viper.GetString("credentials.app.client-id"),
		ClientSecret: viper.GetString("credentials.app.client-secret"),
		Limit:        viper.GetInt32("subreddit.submissions.limit"),
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
	filenameRegex := regexp.MustCompile("[^/]*$")

	os.Mkdir("hori", os.ModePerm)
	os.Mkdir("vert", os.ModePerm)

	fetchImage := func(url string, abort chan error) {
		matches := filenameRegex.FindAllString(url, 1)
		if len(matches) == 0 {
			abort <- fmt.Errorf("No match for regex")
			return
		}

		filename := matches[0]
		file, err := os.Create(filename)
		if err != nil {
			abort <- fmt.Errorf("Could not create file %s", filename)
			return
		}
		defer file.Close()

		err = os.Chmod(filename, os.ModePerm)
		if err != nil {
			abort <- err
			return
		}

		resp, err := r.client.Head(url)
		if err != nil {
			abort <- fmt.Errorf("could not get HEAD")
			return
		}
		contentType := resp.Header.Get("content-type")

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Getting image %s, length: %s, type: %s", url, resp.Header.Get("Content-Length"), contentType))

		var codec imageCodec
		if contentType == "image/jpeg" {
			codec = JPEG
		} else if contentType == "image/png" {
			codec = PNG
		}

		resp, err = r.client.Get(url)
		if err != nil {
			abort <- fmt.Errorf("Could not gete content length")
			return
		}

		defer resp.Body.Close()
		_, err = io.Copy(file, resp.Body)
		if err != nil {
			abort <- fmt.Errorf("No match for regex")
			return
		}

		aspectRatio, err := getImageAspectRatio(filename, codec)
		if err != nil {
			abort <- fmt.Errorf("No match for regex")
			return
		}
		sb.WriteString(fmt.Sprintf(", aspect ratio: %f", aspectRatio))

		var newPath string
		if aspectRatio > 1.0 {
			newPath = fmt.Sprintf("hori/%s", filename)
		} else {
			newPath = fmt.Sprintf("vert/%s", filename)
		}

		err = os.Rename(filename, newPath)
		if err != nil {
			abort <- err
			return
		}
		fmt.Println(sb.String())

		abort <- nil
	}

	abort := make(chan error)
	for _, url := range validURLs {
		go fetchImage(url, abort)
	}

	for i := 0; i < len(validURLs); i++ {
		if (<-abort) != nil {
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

type imageCodec string

const (
	JPEG imageCodec = "jpeg"
	PNG  imageCodec = "png"
)

func getImageAspectRatio(filename string, codec imageCodec) (float64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0.0, err
	}

	switch codec {
	case JPEG:
		return getJPEGAspectRatio(file)
	case PNG:
		return getPNGAspectRatio(file)
	default:
		return 0.0, errors.New("unsupported file type")
	}
}

func getJPEGAspectRatio(file *os.File) (float64, error) {
	imageCfg, err := jpeg.DecodeConfig(file)
	if err != nil {
		return 0.0, err
	}

	return float64(imageCfg.Width) / float64(imageCfg.Height), nil
}

func getPNGAspectRatio(file *os.File) (float64, error) {
	imageCfg, err := png.DecodeConfig(file)
	if err != nil {
		return 0.0, err
	}

	return float64(imageCfg.Width) / float64(imageCfg.Height), nil
}
