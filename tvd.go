package main

// Based on concat by ArneVogel
// https://github.com/ArneVogel/concat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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

	// Download chunks
	chunks, tempDir, err := downloadChunks(chunks, cfg.VodID, cfg.Workers)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Build output filename & path
	outFile, err := buildOutFilePath(cfg.VodID, cfg.StartTime, clipDur, cfg.FilePrefix, cfg.OutputFolder)
	if err != nil {
		return err
	}

	// Combine chunks
	err = combineChunks(chunks, outFile)
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
	var ql = make(map[string]string)

	url := fmt.Sprintf("http://usher.twitch.tv/vod/%d?nauthsig=%s&nauth=%s&allow_source=true", vodID, atr.Sig, atr.Token)
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
		endAt, err = timeInputToSeconds(endTime)
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

func downloadChunks(chunks []Chunk, vodID, workers int) ([]Chunk, string, error) {
	tempDir, err := ioutil.TempDir("", fmt.Sprintf("tvd_%d", vodID))
	if err != nil {
		return nil, "", err
	}

	jobs := make(chan Chunk, len(chunks))
	results := make(chan string, len(chunks))

	// Spin up workers
	for w := 1; w < workers; w++ {
		go downloadWorker(w, jobs, results)
	}

	// Fill job queue with chunks
	for i, c := range chunks {
		c.Path = filepath.Join(tempDir, c.Name)
		chunks[i] = c
		jobs <- c
	}
	close(jobs)

	// Wait for results to come in
	for r := 0; r < len(chunks); r++ {
		res := <-results
		if len(res) != 0 {
			close(results)
			return nil, "", fmt.Errorf("error: a worker returned an error: %s", res)
		}
	}

	return chunks, tempDir, nil
}

func downloadWorker(id int, chunks <-chan Chunk, results chan<- string) {
	for chunk := range chunks {
		err := downloadChunk(chunk)
		res := ""
		if err != nil {
			res = err.Error()
		}
		results <- res
	}
}

func downloadChunk(c Chunk) error {
	resp, err := http.Get(c.URL.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	chunkFile, err := os.Create(c.Path)
	if err != nil {
		return err
	}
	defer chunkFile.Close()

	_, err = io.Copy(chunkFile, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func buildOutFilePath(vodID int, startTime string, dur int, prefix string, folder string) (string, error) {
	startAt, err := timeInputToSeconds(startTime)
	if err != nil {
		return "", err
	}
	startTime = secondsToTimeMask(startAt)
	endAt := startAt + dur
	endTime := secondsToTimeMask(endAt)

	filename := fmt.Sprintf("%d-%s-%s", vodID, startTime, endTime)

	if len(prefix) > 0 {
		filename = prefix + filename
	}

	if len(folder) > 0 {
		filename = filepath.Join(folder, filename)
	}

	return filename, nil
}

func combineChunks(chunks []Chunk, outfile string) error {
	concat := "concat:"
	for _, c := range chunks {
		concat += c.Path + "|"
	}
	concat = string(concat[0 : len(concat)-1])

	args := []string{"-i", concat, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-fflags", "+genpts", outfile}

	ffmpeg := "ffmpeg"
	if runtime.GOOS == "windows" {
		ffmpeg += ".exe"
	}
	cmd := exec.Command(ffmpeg, args...)
	var errbuf bytes.Buffer
	cmd.Stderr = &errbuf
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf(errbuf.String())
	}

	return nil
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

func secondsToTimeMask(s int) string {
	hours := s / 3600
	minutes := s % 3600 / 60
	seconds := s % 60
	return fmt.Sprintf("%0dh%0dm%0ds", hours, minutes, seconds)
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
