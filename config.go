package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"regexp"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

// Config represents a config object containing everything needed to download a VOD
type Config struct {
	AuthToken    string
	ClientID     string
	Quality      string
	StartTime    string
	StartSec     int
	EndTime      string
	EndSec       int
	Length       string
	VodID        int
	FilePrefix   string
	OutputFolder string
	Workers      int
}

// Privatize returns a copy of the struct with the ClientID field censored (e.g. for logging)
func (c Config) Privatize() Config {
	c2 := c
	c2.AuthToken = "********"
	c2.ClientID = "********"
	return c2
}

// Update replaces any config values in the base object with those present in the passed argument
func (c *Config) Update(c2 Config) {
	if c2.AuthToken != "" {
		c.AuthToken = c2.AuthToken
	}
	if c2.ClientID != "" {
		c.ClientID = c2.ClientID
	}
	if c2.Quality != "" {
		c.Quality = c2.Quality
	}
	if c2.StartTime != "" {
		c.StartTime = c2.StartTime
	}
	if c2.EndTime != "" {
		c.EndTime = c2.EndTime
	}
	if c2.Length != "" {
		c.EndTime = c2.Length
	}
	if c2.VodID != 0 {
		c.VodID = c2.VodID
	}
	if c2.FilePrefix != "" {
		c.FilePrefix = c2.FilePrefix
	}
	if c2.OutputFolder != "" {
		c.OutputFolder = c2.OutputFolder
	}
	if c2.Workers != 0 {
		c.Workers = c2.Workers
	}
}

// Validate checks if the config object appears valid. Required attributes must
// be present and appear correct. Optional values are validated if present.
//
// Currently, "OutputFolder" is not validated (needs logic to support Windows paths)
func (c Config) Validate() error {
	if len(c.AuthToken) == 0 {
		return fmt.Errorf("error: AuthToken missing")
	}
	if len(c.ClientID) == 0 {
		return fmt.Errorf("error: ClientID missing")
	}

	if c.VodID < 1 {
		return fmt.Errorf("error: VodID missing")
	}

	timePattern := `\d+ \d+ \d+`
	timeRegex := regexp.MustCompile(timePattern)
	if c.StartTime != "start" && !timeRegex.MatchString(c.StartTime) {
		return fmt.Errorf("error: StartTime must be 'start' or in format '%s'; got '%s'", timePattern, c.StartTime)
	}
	if c.EndTime == "" && c.Length == "" {
		return errors.New("error: must specify either EndTime or Length")
	}
	if c.Length == "" && c.EndTime != "end" && !timeRegex.MatchString(c.EndTime) {
		return fmt.Errorf("error: EndTime must be 'end' or in format '%s'; got '%s'", timePattern, c.EndTime)
	}
	if c.EndTime == "" && c.Length != "full" && !timeRegex.MatchString(c.Length) {
		return fmt.Errorf("error: Length must be 'full' or in format '%s'; got '%s'", timePattern, c.Length)
	}

	qualityPattern := `\d{3,4}p[36]0`
	qualityRegex := regexp.MustCompile(qualityPattern)
	if c.Quality != "best" && c.Quality != "chunked" && !qualityRegex.MatchString(c.Quality) {
		return fmt.Errorf("error: Quality must be 'best', 'chunked', or in format '%s'; got '%s'", qualityPattern, c.Quality)
	}

	if c.FilePrefix != "" && !isValidFilename(c.FilePrefix) {
		return fmt.Errorf("error: FilePrefix contains invalid characters; got '%s'", c.FilePrefix)
	}

	if c.Workers < 1 {
		return fmt.Errorf("error: Worker must be an integer greater than 0; got '%d'", c.Workers)
	}

	return nil
}

// ResolveEndTime is a hacky way of supporting organically support the new
// 'length' option. If that prop is included, it is used to calculate & update
// EndTime prop (it is intended to override)
func (c *Config) ResolveEndTime() error {
	startAt, err := timeInputToSeconds(c.StartTime)
	if err != nil {
		return err
	}
	c.StartSec = startAt

	if c.Length != "" {
		if c.Length == "full" {
			c.EndSec = -1
		} else {
			endAt, err := timeInputToSeconds(c.Length)
			if err != nil {
				return err
			}
			c.EndSec = startAt + endAt
		}
	} else {
		if c.EndTime == "end" {
			c.EndSec = -1
		} else {
			endAt, err := timeInputToSeconds(c.EndTime)
			if err != nil {
				return err
			}
			c.EndSec = endAt
		}
	}

	return nil
}

func loadConfig(f string) (Config, error) {
	log.Printf("loading config file <%s>\n", f)
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

	if *authToken != "" {
		config.AuthToken = *authToken
	}
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
