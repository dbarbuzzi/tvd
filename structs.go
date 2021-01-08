package main

import (
	"log"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func isValidFilename(fn string) bool {
	log.Printf("[isValidFilename] validating: %s\n", fn)
	if runtime.GOOS != "windows" {
		return true
	}

	// source: https://msdn.microsoft.com/en-us/library/aa365247
	// first, check for bad characters
	log.Println("[isValidFilename] checking for bad characters")
	badChars := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, badChar := range badChars {
		if strings.Contains(fn, badChar) {
			return false
		}
	}
	// next, check for bad names
	log.Println("[isValidFilename] checking for bad names")
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
	log.Println("[isValidFilename] checking final character")
	if string(fn[len(fn)-1]) == " " || string(fn[len(fn)-1]) == "." {
		return false
	}

	return true
}

// AuthTokenResponse represents the (happy) JSON response to a token request call
type AuthTokenResponse struct {
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
