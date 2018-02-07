package main

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

// AccessTokenResponse represents the (happy) JSON response to a token request call
type AccessTokenResponse struct {
	Sig   string `json:"token"`
	Token string `json:"sig"`
}
