package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Based on concat by ArneVogel
// https://github.com/ArneVogel/concat

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

	// go get it!
}

func loadConfig(f string) Config {
	config := Config{
		Quality: "best",
		Workers: 4,
	}

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

	return config
}
