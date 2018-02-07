package main

// Based on concat by ArneVogel
// https://github.com/ArneVogel/concat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
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

	// Get chunk list for desired stream option
	streamURL, ok := ql[cfg.Quality]
	if !ok {
		i := 0
		options := make([]string, len(ql))
		for k := range ql {
			options[i] = k
			i++
		}
		return fmt.Errorf("error: quality %s not available in list %+v", cfg.Quality, options)
	}
	chunks, chunkDur, err := getChunks(streamURL)
	if err != nil {
		return err
	}

	// Prune chunk list to those needed for requested stream time
	chunks, clipDur, err := pruneChunks(chunks, cfg.StartTime, cfg.EndTime, chunkDur)
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

func getChunks(streamURL string) ([]Chunk, int, error) {
	var chunks []Chunk

	respData, err := readURL(streamURL)
	if err != nil {
		return nil, 0, err
	}

	re := regexp.MustCompile(`#EXTINF:(\d+\.\d+),\n(.*?)\n`)
	matches := re.FindAllStringSubmatch(string(respData), -1)

	// "safe" to ignore because we already fetched it
	baseURL, _ := url.Parse(streamURL)
	for _, match := range matches {
		// "safe" to ignore error due to regex capture group
		length, _ := strconv.ParseFloat(match[1], 64)
		// "safe" to ignore error ... due to capture group?
		chunkURL, _ := url.Parse(match[2])
		chunkURL = baseURL.ResolveReference(chunkURL)
		chunks = append(chunks, Chunk{Name: match[2], Length: length, URL: chunkURL})
	}

	re = regexp.MustCompile(`#EXT-X-TARGETDURATION:(\d+)\n`)
	match := re.FindStringSubmatch(string(respData))
	chunkDur, _ := strconv.Atoi(match[1])

	return chunks, chunkDur, nil
}

func pruneChunks(chunks []Chunk, startTime, endTime string, duration int) ([]Chunk, int, error) {
	startAt, err := timeInputToSeconds(startTime)
	if err != nil {
		return nil, 0.0, err
	}
	startAt = startAt / duration
	// assume "end", work to determine actual end chunk if different
	endAt := len(chunks)
	if endTime != "end" {
		endAt, err := timeInputToSeconds(endTime)
		if err != nil {
			return nil, 0.0, err
		}
		endAt = endAt / duration
	}

	res := chunks[startAt:endAt]

	actualDuration := 0.0
	for _, c := range res {
		actualDuration += c.Length
	}

	return res, int(actualDuration), nil
}

func timeInputToSeconds(t string) (int, error) {
	entries := strings.Split(t, " ")
	if len(entries) != 3 {
		return 0, fmt.Errorf("error: time input must be in format \"H M S\", got '%s'", t)
	}

	hours, err := strconv.Atoi(entries[0])
	minutes, err := strconv.Atoi(entries[1])
	seconds, err := strconv.Atoi(entries[2])
	if err != nil {
		return 0, fmt.Errorf("error: all time inputs must be integers, got '%s'", t)
	}

	s := hours*3600 + minutes*60 + seconds
	return s, nil
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
