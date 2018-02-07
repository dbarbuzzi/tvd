package main

// Based on concat by ArneVogel
// https://github.com/ArneVogel/concat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

func main() {
	// Initialize logging to file
	now := time.Now()
	logfilepath := fmt.Sprintf("logs/%s.log", now.Format("20060102-030405"))
	logfile, err := os.OpenFile(logfilepath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal("Failed to create log file")
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	// config file
	config := loadConfig("config.toml")

	// flags (todo)

	// go get it!
	err = DownloadVOD(config)
	if err != nil {
		log.Fatal(err)
	}
}

// DownloadVOD downloads a VOD based on the various info passed in the config
func DownloadVOD(cfg Config) error {
	// Get an access token
	atr, err := getAccessToken(cfg.VodID, cfg.ClientID)
	if err != nil {
		return err
	}

	// Use access token to get m3u (list of vod stream options)
	ql, err := getStreamOptions(cfg.VodID, atr)
	if err != nil {
		return err
	}

	return nil
}

func getAccessToken(vodID int, clientID string) (AccessTokenResponse, error) {
	var atr AccessTokenResponse
	url := fmt.Sprintf("https://api.twitch.tv/api/vods/%d/access_token?client_id=%s", vodID, clientID)
	respData, err := readURL(url)
	if err != nil {
		return atr, err
	}

	err = json.Unmarshal(respData, &atr)
	if err != nil {
		return atr, err
	}
	if len(atr.Sig) == 0 || len(atr.Token) == 0 {
		return atr, fmt.Errorf("error: sig and/or token were empty: %+v", atr)
	}

	return atr, nil
}

func getStreamOptions(vodID int, atr AccessTokenResponse) (map[string]string, error) {
	var ql map[string]string

	url := fmt.Sprintf("https://usher.twitch.tv/vod/%d?authsig=%s&nauth=%s&allow_source=true", vodID, atr.Sig, atr.Token)
	respData, err := readURL(url)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`BANDWIDTH=(\d+),.*?VIDEO="(.*?)"\n(.*?)\n`)
	matches := re.FindAllStringSubmatch(string(respData), -1)
	bestBandwidth := 0
	for _, match := range matches {
		ql[match[2]] = match[3]
		// "safe" to ignore error as regex only matches digits for this capture grouop
		bandwidth, _ := strconv.Atoi(match[1])
		if bandwidth > bestBandwidth {
			bestBandwidth = bandwidth
			ql["best"] = match[3]
		}
	}

	return ql, nil
}

func loadConfig(f string) Config {
	config := Config{
		Quality: "best",
		Workers: 4,
	}

	configData, err := ioutil.ReadFile(f)
	if err != nil {
		fmt.Println("W: Failed to load config.toml")
		return config
	}

	err = toml.Unmarshal(configData, &config)
	if err != nil {
		fmt.Println("W: Failed to parse config.toml")
		return config
	}

	return config
}

func readURL(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
