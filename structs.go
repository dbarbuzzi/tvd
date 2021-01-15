package main

import (
	"encoding/json"
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

// AuthGQLPayload represents the payload sent to the GQL endpoint to get the
// auth token and signature
type AuthGQLPayload struct {
	OperationName string `json:"operationName"`
	Query         string `json:"query"`
	Variables     struct {
		IsLive     bool   `json:"isLive"`
		IsVod      bool   `json:"isVod"`
		Login      string `json:"login"`
		PlayerType string `json:"playerType"`
		VodID      string `json:"vodID"`
	} `json:"variables"`
}

func generateAuthPayload(vodID string) ([]byte, error) {
	ap := AuthGQLPayload{
		OperationName: "PlaybackAccessToken_Template",
		Query:         "query PlaybackAccessToken_Template($login: String!, $isLive: Boolean!, $vodID: ID!, $isVod: Boolean!, $playerType: String!) {  streamPlaybackAccessToken(channelName: $login, params: {platform: \"web\", playerBackend: \"mediaplayer\", playerType: $playerType}) @include(if: $isLive) {    value    signature    __typename  }  videoPlaybackAccessToken(id: $vodID, params: {platform: \"web\", playerBackend: \"mediaplayer\", playerType: $playerType}) @include(if: $isVod) {    value    signature    __typename  }}",
	}
	ap.Variables.IsLive = false
	ap.Variables.IsVod = true
	ap.Variables.PlayerType = "site"
	ap.Variables.VodID = vodID
	return json.Marshal(ap)
}

// AuthGQLPayload represents the response from to the GQL endpoint containing
// the auth token and signature
type AuthGQLResponse struct {
	Data struct {
		VideoPlaybackAccessToken struct {
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"videoPlaybackAccessToken"`
	} `json:"data"`
}
