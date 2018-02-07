# tvd

tvd (**T**witch **V**OD **D**ownloader) is a command-line tool to download VODs from Twitch.tv. It is modeled after [concat](https://github.com/ArneVogel/concat) by [ArneVogel](https://github.com/ArneVogel).

## Prerequisites

* You must have [ffmpeg](https://www.ffmpeg.org/) available on your path regardless of platform.
* You must register a new app on the Twitch Dev site to get your own client ID

## Download

Visit the [releases](/dbarbuzzi/tvd/releases) page to download the latest binary for your platform.

## Usage

Currently, no command-line args are supported. This is a top-priority future enhancement. In the meantime, everything is controlled through the config file. Create a file named `tvd-config.toml` and copy the contents from `config-sample.toml` as a baseline. Marked values are optional and can be omitted to use defaults.

The accepted values are:

* `ClientID` - your Twitch app’s client ID
* `Quality` (optional) - desired quality (e.g. “720p60”, “480p30”); can use “best” for best available (default: "best")
* `StartTime` – start time in the format "HOURS MINUTES SECONDS" (e.g. "1 24 35" is 1h24m35s)
* `EndTime` – end time in the same format as above (also supported: "end")
* `VodID` – ID of the VOD to be downloaded
* `FilePrefix` (optional) – Prefix for the output filename, include your own separator (default: none)
* `OutputFolder` (optional) – Full path to the folder to save the file (e.g. `/Users/username/downloads` or `C:\Users\username\`) (default: current working directory)
* `Workers` (optional) – Number of concurrent downloads (default: 4)
