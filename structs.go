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
