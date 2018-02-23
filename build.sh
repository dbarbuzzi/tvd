#!/bin/bash

# make sure client id is available as environment variable
if [ -z ${TWITCH_CLIENT_ID+x} ]; then
    echo "\$TWITCH_CLIENT_ID is not set; canceling build."
    exit 1
fi

# get version based on git tags
TVD_VERSION=`git describe --tags`

# clean dist folder
rm -rf dist/*

# build the binaries
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.ClientID=${TWITCH_CLIENT_ID} -X main.Version=${TVD_VERSION}" -o dist/darwin-amd64/tvd
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.ClientID=${TWITCH_CLIENT_ID} -X main.Version=${TVD_VERSION}" -o dist/linux-amd64/tvd
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.ClientID=${TWITCH_CLIENT_ID} -X main.Version=${TVD_VERSION}" -o dist/windows-amd64/tvd.exe

# package the binaries
cd dist
for dir in ./*/; do
    cleanDir="${dir%/}"
    cleanDir="${cleanDir##*/}"
    cd $dir
    for file in "./"*; do
        zip -r "../$cleanDir.zip" $file
    done
    cd ..
done
