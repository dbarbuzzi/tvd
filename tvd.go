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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/grafov/m3u8"
	"github.com/schollz/progressbar/v3"
	"gopkg.in/alecthomas/kingpin.v2"
)

// vars intended to be populated via ldflags during build
var (
	// ClientID is provided by the Twitch API when registering an application
	ClientID string
	// Version is the build/release version
	version = "dev"
	commit  = "n/a"
	date    = "n/a"

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

	prefix = kingpin.Flag("prefix", "Prefix for the output filename").Short('p').String()
	folder = kingpin.Flag("folder", "Target folder for saved file (default: current dir)").Short('f').String()
	// outFile = kingpin.Flag("output", "NOT YET IMPLEMENTED").Short('o').String()
)

func main() {
	// parse command-line input
	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Version(fmt.Sprintf("%s (commit %s; built %s)", version, commit, date))
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
	log.Printf("default config: %+v\n", config.Privatize())

	// config file
	configfile := DefaultConfigPath
	if *configFile != "" {
		configfile = *configFile
	}
	fileConfig, err := loadConfig(configfile)
	if err != nil {
		if configfile == DefaultConfigPath {
			log.Print("creating default config file")
			innerErr := createDefaultConfigFile()
			if innerErr != nil {
				fmt.Println(err)
				log.Fatalln(err)
			}
		} else {
			fmt.Println(err)
			log.Fatalln(err)
		}
	}
	log.Printf("config from file: %+v\n", config.Privatize())
	config.Update(fileConfig)
	log.Printf("config after merging config file: %+v\n", config.Privatize())

	// flags (todo)
	flagConfig, err := buildConfigFromFlags()
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}
	log.Printf("config from cli args/flags: %+v\n", config.Privatize())
	config.Update(flagConfig)
	log.Printf("config after merging cli args/flags: %+v\n", config.Privatize())

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
	log.Printf("final config: %+v\n", config.Privatize())

	// go get it!
	err = DownloadVOD(config)
	if err != nil {
		fmt.Println(err)
		log.Fatalln(err)
	}
}

func createDefaultConfigFile() error {
	err := os.MkdirAll(DefaultConfigFolder, os.ModePerm)
	if err != nil {
		return err
	}

	configFile, err := os.Create(DefaultConfigPath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	// TODO: marshal default config toml to file
	e := toml.NewEncoder(configFile)
	err = e.Encode(DefaultConfig)
	return err
}

// DownloadVOD downloads a VOD based on the various info passed in the config
func DownloadVOD(cfg Config) error {
	fmt.Println("Fetching access token")
	ar, err := getAccessData(cfg.VodID, cfg.ClientID)
	if err != nil {
		return err
	}

	fmt.Println("Fetching VOD stream options")
	ql, err := getStreamOptions(cfg.VodID, ar)
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
		fmt.Println("Cleaning up temp files")
		err = os.RemoveAll(tempDir)
		if err != nil {
			fmt.Printf("Failed to remove tempdir <%s>\n", tempDir)
			log.Fatalln(err)
		}
	}()

	fmt.Println("Building output filepath")
	outFile, err := buildOutFilePath(cfg.VodID, cfg.StartSec, clipDur, cfg.FilePrefix, cfg.OutputFolder)
	if err != nil {
		return err
	}

	fmt.Printf("Combining chunks to %s\n", outFile)
	err = combineChunks(chunks, outFile)
	if err != nil {
		return err
	}

	return nil
}

func getAccessData(vodID int, clientID string) (AuthGQLResponse, error) {
	log.Printf("[getAuthToken] vodID=%d\n", vodID)
	var ar AuthGQLResponse

	ap, err := generateAuthPayload(strconv.Itoa(vodID))
	if err != nil {
		return ar, err
	}

	url := "https://gql.twitch.tv/gql"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(ap))
	if err != nil {
		return ar, err
	}
	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Content-Type", "text/plain; charset=UTF-8")

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ar, err
	}
	defer func() {
		err = rsp.Body.Close()
		if err != nil {
			fmt.Printf("error closing URL body for <%s>: %s", url, err.Error())
			log.Println(err)
		}
	}()

	rspData, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return ar, err
	}

	err = json.Unmarshal(rspData, &ar)
	if err != nil {
		return ar, err
	}
	if len(ar.Data.VideoPlaybackAccessToken.Signature) == 0 || len(ar.Data.VideoPlaybackAccessToken.Value) == 0 {
		log.Printf("response: %s\n", rspData)
		return ar, fmt.Errorf("error: sig and/or token were empty: %+v", ar)
	}

	log.Printf("access token: %+v\n", ar)

	return ar, nil
}

func getStreamOptions(vodID int, ar AuthGQLResponse) (map[string]string, error) {
	log.Printf("[getStreamOptions] vodID=%d, ar=%+v\n", vodID, ar)
	var ql = make(map[string]string)

	url := fmt.Sprintf(
		"https://usher.ttvnw.net/vod/%d.m3u8?allow_source=true&sig=%s&token=%s",
		vodID,
		ar.Data.VideoPlaybackAccessToken.Signature,
		ar.Data.VideoPlaybackAccessToken.Value,
	)
	rsp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = rsp.Body.Close()
		if err != nil {
			fmt.Printf("error closing URL body for <%s>: %s", url, err.Error())
			log.Println(err)
		}
	}()

	p, listType, err := m3u8.DecodeFrom(rsp.Body, true)
	if err != nil {
		return nil, err
	}

	switch listType {
	case m3u8.MASTER:
		masterPl := p.(*m3u8.MasterPlaylist)
		var bestBandwidth uint32
		for _, v := range masterPl.Variants {
			ql[v.Resolution] = v.URI
			if v.Bandwidth > bestBandwidth {
				bestBandwidth = v.Bandwidth
				ql["best"] = v.URI
			}

		}
	default:
		return nil, fmt.Errorf("m3u8 playlist was not the expected 'master' format")
	}

	log.Printf("qualities options found: %+v\n", ql)

	return ql, nil
}

func getChunks(streamURL string) ([]Chunk, int, error) {
	var chunks []Chunk
	var chunkDur int

	rsp, err := http.Get(streamURL)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		err = rsp.Body.Close()
		if err != nil {
			fmt.Printf("error closing URL body for <%s>: %s", streamURL, err.Error())
			log.Println(err)
		}
	}()

	p, listType, err := m3u8.DecodeFrom(rsp.Body, true)
	if err != nil {
		return nil, 0, err
	}

	switch listType {
	case m3u8.MEDIA:
		mediaPl := p.(*m3u8.MediaPlaylist)

		chunkDur = int(mediaPl.TargetDuration)
		log.Printf("target chunk duration: %d", chunkDur)

		// "safe" to ignore - previously fetched
		baseURL, _ := url.Parse(streamURL)
		for i := 0; i < int(mediaPl.Count()); i++ {
			// "safe" to ignore - per format spec
			s := mediaPl.Segments[i]
			chunkPath, _ := url.Parse(s.URI)
			chunkURL := baseURL.ResolveReference(chunkPath)
			chunks = append(chunks, Chunk{Name: s.URI, Length: s.Duration, URL: chunkURL})
		}
	default:
		return nil, 0, fmt.Errorf("m3u8 playlist was not the expected 'media' format")
	}

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
	for w := 1; w <= workers; w++ {
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

	// Wait for results to come in
	log.Printf("waiting for results from workers")
	bar := progressbar.Default(int64(len(chunks)))
	for r := 0; r < len(chunks); r++ {
		res := <-results
		// below error-catching is untested... cross your fingers
		if len(res) != 0 {
			close(results)
			return nil, "", fmt.Errorf("error: a worker returned an error: %s", res)
		}
		err := bar.Add(1)
		if err != nil {
			return nil, "", fmt.Errorf("error: failed to increment progress bar: %w", err)
		}
	}
	err = bar.Finish()
	if err != nil {
		return nil, "", fmt.Errorf("error: failed to finalize progress bar: %w", err)
	}

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
	of, err := os.Create(outfile)
	if err != nil {
		return err
	}
	defer of.Close()

	bar := progressbar.Default(int64(len(chunks)))
	for _, c := range chunks {
		cf, err := os.Open(c.Path)
		if err != nil {
			return err
		}

		_, err = io.Copy(of, cf)
		cf.Close()
		if err != nil {
			return err
		}
		err = bar.Add(1)
		if err != nil {
			return fmt.Errorf("error: failed to increment progress bar: %w", err)
		}
	}
	err = bar.Finish()
	if err != nil {
		return fmt.Errorf("error: failed to increment progress bar: %w", err)
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
