package main

// Based on https://github.com/ArneVogel/concat

import (
	"bytes"
	"encoding/json"
	"flag"
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
	"gopkg.in/cheggaaa/pb.v1"
)

// below block's vars are populated via ldflags during build
var (
	// ClientID is provided by the Twitch API when registering an application
	ClientID string
	// Version is the release version
	Version string
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
	flagConfig, err := parseFlags()
	if err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}
	config.Update(flagConfig)
	log.Printf("Final config: %+v\n", config)
	err = config.Validate()
	if err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}

	// go get it!
	err = DownloadVOD(config)
	if err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}
}

// DownloadVOD downloads a VOD based on the various info passed in the config
func DownloadVOD(cfg Config) error {
	fmt.Println("Fetching access token")
	atr, err := getAccessToken(cfg.VodID, cfg.ClientID)
	if err != nil {
		return err
	}

	fmt.Println("Fetching VOD stream options")
	ql, err := getStreamOptions(cfg.VodID, atr)
	if err != nil {
		return err
	}

	fmt.Println("Picking selected quality")
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
	fmt.Println("Fetching chunk list")
	chunks, chunkDur, err := getChunks(streamURL)
	if err != nil {
		return err
	}

	fmt.Println("Pruning chunk list")
	chunks, clipDur, err := pruneChunks(chunks, cfg.StartTime, cfg.EndTime, chunkDur)
	if err != nil {
		return err
	}

	fmt.Println("Downloading chunks")
	chunks, tempDir, err := downloadChunks(chunks, cfg.VodID, cfg.Workers)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fmt.Println("Building output filepath")
	outFile, err := buildOutFilePath(cfg.VodID, cfg.StartTime, clipDur, cfg.FilePrefix, cfg.OutputFolder)
	if err != nil {
		return err
	}

	fmt.Println("Combining chunks")
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

	log.Printf("Access Token:\n    Sig:   %s\n    Token: %s\n", atr.Sig, atr.Token)

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
	if len(matches) == 0 {
		log.Printf("Response for m3u:\n%s\n", respData)
		return nil, fmt.Errorf("error: no matches found")
	}

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

	log.Printf("Qualities options found:\n")
	for k, v := range ql {
		log.Printf("    %s: %s\n", k, v)
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
	log.Printf("Found %d chunks", len(chunks))

	re = regexp.MustCompile(`#EXT-X-TARGETDURATION:(\d+)\n`)
	match := re.FindStringSubmatch(string(respData))
	chunkDur, _ := strconv.Atoi(match[1])
	log.Printf("Target chunk duration: %d", chunkDur)

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

	log.Printf("Start at chunk:          %d\n", startAt)
	log.Printf("End at chunk:            %d\n", endAt)

	res := chunks[startAt:endAt]

	log.Printf("Number of pruned chunks: %d\n", len(res))

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

	bar := pb.StartNew(len(chunks))

	// Wait for results to come in
	for r := 0; r < len(chunks); r++ {
		res := <-results
		// below error-catching is untested... cross your fingers
		if len(res) != 0 {
			close(results)
			return nil, "", fmt.Errorf("error: a worker returned an error: %s", res)
		}
		bar.Increment()
	}
	bar.Finish()

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

	filename := fmt.Sprintf("%d-%s-%s.mp4", vodID, startTime, endTime)

	if len(prefix) > 0 {
		filename = prefix + filename
	}

	if len(folder) > 0 {
		filename = filepath.Join(folder, filename)
	}

	log.Printf("Output file: %s\n", filename)
	return filename, nil
}

func combineChunks(chunks []Chunk, outfile string) error {
	ffmpeg := "ffmpeg"
	if runtime.GOOS == "windows" {
		ffmpeg += ".exe"
	}
	log.Printf("ffmpeg command: %s\n", ffmpeg)

	concat := "concat:"
	for _, c := range chunks {
		concat += c.Path + "|"
	}
	concat = string(concat[0 : len(concat)-1])

	args := []string{"-i", concat, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-fflags", "+genpts", outfile}
	log.Printf("ffmpeg args:\n%+v\n", args)
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
	if err != nil {
		return 0, fmt.Errorf("error: hours must be integer, got '%s' ('%s')", entries[0], t)
	}
	minutes, err := strconv.Atoi(entries[1])
	if err != nil {
		return 0, fmt.Errorf("error: minutes must be integer, got '%s' ('%s')", entries[1], t)
	}
	seconds, err := strconv.Atoi(entries[2])
	if err != nil {
		return 0, fmt.Errorf("error: seconds must be integer, got '%s' ('%s')", entries[2], t)
	}

	s := hours*3600 + minutes*60 + seconds
	log.Printf("Converted '%s' to %d seconds\n", t, s)
	return s, nil
}

func secondsToTimeMask(s int) string {
	hours := s / 3600
	minutes := s % 3600 / 60
	seconds := s % 60
	res := fmt.Sprintf("%02dh%02dm%02ds", hours, minutes, seconds)
	log.Printf("Masked %d seconds as '%s'\n", s, res)
	return res
}

func loadConfig(f string) Config {
	config := Config{
		ClientID: ClientID,
		Quality:  "best",
		Workers:  4,
	}
	log.Printf("Default config: %+v\n", config)

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

	log.Printf("Config after parsing config file: %+v\n", config)
	return config
}

func parseFlags() (Config, error) {
	var config Config

	client := flag.String("client", "", "client ID of registered Twitch App")
	quality := flag.String("quality", "", "desired quality ('720p30', 'best', etc.)")
	startTime := flag.String("start", "", "start time (e.g. '0 15 0' to start at 15 minute mark)")
	endTime := flag.String("end", "", "end time (e.g. '0 30 0' to end at 30 minute mark)")
	prefix := flag.String("prefix", "", "optional prefix for the output file's name")
	folder := flag.String("folder", "", "target folder for output file (default: current dir)")
	workers := flag.Int("workers", 0, "number of concurrent downloads(default: 4) ")
	flag.Parse()

	if *client != "" {
		config.ClientID = *client
	}
	if *quality != "" {
		config.Quality = *quality
	}
	if *startTime != "" {
		config.StartTime = *startTime
	}
	if *endTime != "" {
		config.EndTime = *endTime
	}
	if *prefix != "" {
		config.FilePrefix = *prefix
	}
	if *folder != "" {
		config.OutputFolder = *folder
	}
	if *workers != 0 {
		config.Workers = *workers
	}

	if len(flag.Args()) > 0 {
		vID, err := strconv.Atoi(flag.Arg(0))
		if err != nil {
			return config, err
		}
		config.VodID = vID
	}

	log.Printf("Flag config: %+v\n", config)
	return config, nil
}

func readURL(url string) ([]byte, error) {
	log.Printf("Requesting URL: %s\n", url)
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
