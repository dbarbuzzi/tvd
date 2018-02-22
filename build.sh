#!/bin/bash

# make sure client id is available as environment variable
if [ -z ${TWITCH_CLIENT_ID+x} ]; then
    echo "\$TWITCH_CLIENT_ID is not set; canceling build."
    exit 1
fi

# clean dist folder
rm -rf dist/*

# (temporarily) insert client ID into source
sed -i '' s/:CLIENT_ID:/$TWITCH_CLIENT_ID/ tvd.go

# build the binaries
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/darwin-amd64/tvd
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/linux-amd64/tvd
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/windows-amd64/tvd.exe

# undo adding of client ID
sed -i '' s/$TWITCH_CLIENT_ID/:CLIENT_ID:/ tvd.go
