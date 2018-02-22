package main

import (
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
	EndTime      string
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
	if c.EndTime != "end" && !timeRegex.MatchString(c.EndTime) {
		return fmt.Errorf("error: EndTime must be 'end' or in format '%s'; got '%s'", timePattern, c.EndTime)
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
