package main

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Config represents a config object containing everything needed to download a VOD
type Config struct {
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

// Update replaces any config values in the base object with those present in the passed argument
func (c *Config) Update(c2 Config) {
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
	if c.EndTime != "" && c.Length == "" {
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

	if !isValidFilename(c.FilePrefix) {
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

func isValidFilename(fn string) bool {
	if runtime.GOOS != "windows" {
		return true
	}

	// source: https://msdn.microsoft.com/en-us/library/aa365247
	// first, check for bad characters
	badChars := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, badChar := range badChars {
		if strings.Contains(fn, badChar) {
			return false
		}
	}
	// next, check for bad names
	badNames := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}
	for _, badName := range badNames {
		if fn == badName || string(fn[:len(fn)-len(filepath.Ext(fn))]) == badName {
			return false
		}
	}
	// finally, make sure it doesn't end in " " or "."
	if string(fn[len(fn)-1]) == " " || string(fn[len(fn)-1]) == "." {
		return false
	}

	return true
}

// AccessTokenResponse represents the (happy) JSON response to a token request call
type AccessTokenResponse struct {
	Sig   string `json:"sig"`
	Token string `json:"token"`
}

// Chunk represents a video chunk from the m3u
type Chunk struct {
	Name   string
	Length float64
	URL    *url.URL
	Path   string
}
