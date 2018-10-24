package main

// Based on https://github.com/ArneVogel/concat

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

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/cheggaaa/pb.v1"
)

// vars intended to be populated via ldflags during build
var (
	// ClientID is provided by the Twitch API when registering an application
	ClientID string
	// Version is the build/release version
	Version = "dev"

	DefaultConfig = Config{
		ClientID:  ClientID,
		Workers:   4,
		StartTime: "0 0 0",
		EndTime:   "end",
		Quality:   "best",
	}
	DefaultConfigFolder = os.ExpandEnv("${HOME}/.config/tvd/")
	DefaultConfigFile   = "config.toml"
	DefaultConfigPath   = filepath.Join(DefaultConfigFolder, DefaultConfigFile)
)

// command-line args/flags
var (
	clientID   = kingpin.Flag("client", "Twitch app Client ID").Short('C').String()
	workers    = kingpin.Flag("workers", "Max number of concurrent downloads (default: 4)").Short('w').Int()
	configFile = kingpin.Flag("config", "Path to config file (default: $HOME/.config/tvd/config.toml)").Short('c').String()
	logFile    = kingpin.Flag("logfile", "Path to logfile").Short('L').String()

	vodID = kingpin.Arg("vod", "ID of the VOD to download").Default("0").Int()

	quality   = kingpin.Flag("quality", "Desired quality (e.g. '720p30' or 'best')").Short('Q').String()
	startTime = kingpin.Flag("start", "Start time for saved file (e.g. '0 15 0' to start at 15 minute mark)").Short('s').String()
	endTime   = kingpin.Flag("end", "End time for saved file (e.g. '0 30 0' to end at 30 minute mark)").Short('e').String()
	length    = kingpin.Flag("length", "Length from start time, overrides end time (e.g. '0 15 0' for 15 minutes from start time)").Short('l').String()

	prefix  = kingpin.Flag("prefix", "Prefix for the output filename").Short('p').String()
	folder  = kingpin.Flag("folder", "Target folder for saved file (default: current dir)").Short('f').String()
	outFile = kingpin.Flag("output", "NOT YET IMPLEMENTED").Short('o').String()
)

func main() {
	// parse command-line input
	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Version(Version)
	kingpin.Parse()

	// log to file if one is specified, otherwise write to nowhere
	if *logFile != "" {
		logfile, err := os.OpenFile(*logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Fatalln("failed to create log file")
		}
		defer func() {
			err = logfile.Close()
			if err != nil {
				log.Fatalln("failed to close log file")
			}
		}()
		log.SetOutput(logfile)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	// set base config
	config := DefaultConfig
	log.Printf("default config: %+v\n", config.WithoutClientID())

	// config file
	configfile := DefaultConfigPath
	if *configFile != "" {
		configfile = *configFile
	}
	fileConfig, err := loadConfig(configfile)
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}
	log.Printf("config from file: %+v\n", config.WithoutClientID())
	config.Update(fileConfig)
	log.Printf("config after merging config file: %+v\n", config.WithoutClientID())

	// flags (todo)
	flagConfig, err := buildConfigFromFlags()
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}
	log.Printf("config from cli args/flags: %+v\n", config.WithoutClientID())
	config.Update(flagConfig)
	log.Printf("config after merging cli args/flags: %+v\n", config.WithoutClientID())

	// some validation before actually attempting to use the config
	// TODO: Relocate validation to more logical places
	err = config.ResolveEndTime()
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}
	err = config.Validate()
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}

	// go get it!
	err = DownloadVOD(config)
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
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
	chunks, clipDur, err := pruneChunks(chunks, cfg.StartSec, cfg.EndSec, chunkDur)
	if err != nil {
		return err
	}

	fmt.Println("Downloading chunks")
	chunks, tempDir, err := downloadChunks(chunks, cfg.VodID, cfg.Workers)
	if err != nil {
		return err
	}
	defer func() {
		err = os.RemoveAll(tempDir)
		if err != nil {
			fmt.Printf("Failed to remove dir <%s>\n", tempDir)
			log.Fatalln(err)
		}
	}()

	fmt.Println("Building output filepath")
	outFile, err := buildOutFilePath(cfg.VodID, cfg.StartSec, clipDur, cfg.FilePrefix, cfg.OutputFolder)
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

	log.Printf("access token: %+v\n", atr)

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
		log.Printf("response for m3u:\n%s\n", respData)
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

	log.Printf("qualities options found: %+v\n", ql)

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
	log.Printf("found %d chunks", len(chunks))

	re = regexp.MustCompile(`#EXT-X-TARGETDURATION:(\d+)\n`)
	match := re.FindStringSubmatch(string(respData))
	chunkDur, _ := strconv.Atoi(match[1])
	log.Printf("target chunk duration: %d", chunkDur)

	return chunks, chunkDur, nil
}

func pruneChunks(chunks []Chunk, startSec, endSec int, duration int) ([]Chunk, int, error) {
	startAt := startSec / duration
	// assume "end", work to determine actual end chunk if different
	endAt := len(chunks)
	if endSec != -1 {
		endAt = endSec / duration
	}
	if endAt > len(chunks) {
		endAt = len(chunks)
	}

	log.Println("Chunk management:")
	log.Printf("Start at chunk:          %4d\n", startAt)
	log.Printf("End at chunk:            %4d\n", endAt)

	res := chunks[startAt:endAt]

	log.Printf("Number of pruned chunks: %4d\n", len(res))

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
	log.Printf("spinning up %d workers", workers)
	for w := 1; w < workers; w++ {
		go downloadWorker(w, jobs, results)
	}

	// Fill job queue with chunks
	log.Printf("filling job queue with %d chunks", len(chunks))
	for i, c := range chunks {
		c.Path = filepath.Join(tempDir, c.Name)
		chunks[i] = c
		jobs <- c
	}
	close(jobs)

	bar := pb.StartNew(len(chunks))

	// Wait for results to come in
	log.Printf("waiting for results from workers")
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
	log.Printf("worker %02d: spinning up", id)
	for chunk := range chunks {
		log.Printf("worker %02d: received a chunk", id)
		err := downloadChunk(chunk)
		res := ""
		if err != nil {
			log.Printf("worker %02d: chunk download failed", id)
			res = err.Error()
		}
		log.Printf("worker %02d: downloaded a chunk", id)
		results <- res
	}
}

func downloadChunk(c Chunk) error {
	resp, err := http.Get(c.URL.String())
	if err != nil {
		return err
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	chunkFile, err := os.Create(c.Path)
	if err != nil {
		return err
	}
	defer func() {
		err = chunkFile.Close()
		if err != nil {
			fmt.Printf("error closing chunk file %s: %s\n", c.Name, err)
			log.Fatalln(err)
		}
	}()

	_, err = io.Copy(chunkFile, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func buildOutFilePath(vodID int, startAt int, dur int, prefix string, folder string) (string, error) {
	startTime := secondsToTimeMask(startAt)
	endAt := startAt + dur
	endTime := secondsToTimeMask(endAt)

	filename := fmt.Sprintf("%d-%s-%s.mp4", vodID, startTime, endTime)

	if len(prefix) > 0 {
		filename = prefix + filename
	}

	if len(folder) > 0 {
		filename = filepath.Join(folder, filename)
	}

	log.Printf("output file: %s\n", filename)
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
	log.Printf("converted '%s' to %d seconds\n", t, s)
	return s, nil
}

func secondsToTimeMask(s int) string {
	hours := s / 3600
	minutes := s % 3600 / 60
	seconds := s % 60
	res := fmt.Sprintf("%02dh%02dm%02ds", hours, minutes, seconds)
	log.Printf("masked %d seconds as '%s'\n", s, res)
	return res
}

func loadConfig(f string) (Config, error) {
	var config Config

	configData, err := ioutil.ReadFile(f)
	if err != nil {
		return config, errors.Wrap(err, "failed to load config file")
	}

	err = toml.Unmarshal(configData, &config)
	if err != nil {
		return config, errors.Wrap(err, "failed to parse config file")
	}

	return config, nil
}

func buildConfigFromFlags() (Config, error) {
	var config Config

	if *clientID != "" {
		config.ClientID = *clientID
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
	if *length != "" {
		config.Length = *length
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
	if *vodID != 0 {
		config.VodID = *vodID
	}

	return config, nil
}

func readURL(url string) ([]byte, error) {
	log.Printf("requesting URL: %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			fmt.Printf("error closing URL body for <%s>: %s", url, err.Error())
			log.Println(err)
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
