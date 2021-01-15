# tvd

tvd (**T**witch **V**OD **D**ownloader) is a command-line tool to download VODs from Twitch.tv. It is modeled after [concat](https://github.com/ArneVogel/concat) by [ArneVogel](https://github.com/ArneVogel).

## Prerequisites

* If building from source, you must have a client ID with appropriate privileges to query the GQL API for VODs
  * Provided releases have an embedded client ID
* You must have an active auth token from an account sign-in
  * In a browser, sign into your account and get the value of the `auth-token` cookie

## Download

### macOS + Homebrew

If you’re using [Homebrew](https://brew.sh) on macOS, you can use it to install `tvd`:

```bash
brew tap github.com/dbarbuzzi/homebrew-tap
brew install tvd
```

### Windows + Scoop

If you’re using [Scoop](https://scoop.sh) on Windows, you can use it to install `tvd`:

```powershell
scoop bucket add dbarbuzzi https://github.com/dbarbuzzi/scoop-bucket.git
scoop install tvd
```

### Others

Visit the [releases](https://github.com/dbarbuzzi/tvd/releases/latest) page to download the latest release for your platform.

## Usage

Configuration is supported via config file and/org command-line flags. Values in the config file replace any built-in defaults, and values passed via command-line replace built-in/config file values.

### Config file

Using a config file is alternative to command-line arguments. It can be used in conjunction with command-line arguments in which case command-line arguments will take precedence when duplicates are detected. Create a file named `config.toml` and copy the contents from `config-sample.toml` as a baseline. Marked values are optional and can be omitted to use defaults.

The accepted values are:

* `AuthToken` - your login session’s auth token (stored in `auth-token` cookies)
* `ClientID` - your Twitch app’s client ID
* `Quality` (optional) - desired quality (e.g. “720p60”, “480p30”); can use “best” for best available (default: "best")
* `StartTime` – start time in the format "HOURS MINUTES SECONDS" (e.g. "1 24 35" is 1h24m35s)
* `EndTime` – end time in the same format as above (also supported: "end")
* `Length` - duration in same format as `StartTime`/`EndTime` (also supported: "full")
  * Either `EndTime` or `Length` is required. If both are specified, `Length` takes precedence.
* `VodID` – ID of the VOD to be downloaded
* `FilePrefix` (optional) – Prefix for the output filename, include your own separator (default: none)
* `OutputFolder` (optional) – Full path to the folder to save the file (e.g. `/Users/username/downloads` or `C:\Users\username\`) (default: current working directory)
* `Workers` (optional) – Number of concurrent downloads (default: 4)

### Command-line usage

All options supported above are also supported through the command-line under the following flags:

* `auth` => `AuthToken`
* `client` => `ClientID`
* `quality` => `Quality`
* `start` => `StartTime`
* `end` => `EndTime`
* `length` => `Length`
* `prefix` => `FilePrefix`
* `folder` => `OutputFolder`
* `workers` => `Workers`
* `VodID` is passed as an argument, not a flag (e.g. `tvd 123567489`)
