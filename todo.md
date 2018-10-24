# Todo

- [x] Refactor CLI API
  - [x] Consider use-cases (e.g. simplest use-case should allow accepting just a vod ID and sane defaults for *everything* else)
  - [x] Consider using a module (e.g. `kingpin`)
- [x] Logging
  - [x] Support CLI flag to specify logfile path
    - Logging to file only occurs if this flag is set
  - [x] Contents: Potential for additional events to be logged (spinning up workers, worker receiving a task, chunk download complete, worker completing a task)
- [x] Config file
  - [x] File: Default location should be inside folder `$HOME/.config/tvd`
  - [x] File: Support parameter (CLI flag) to specify location
- [ ] Job file
  - [ ] Add support for "job" file which contains config for specific download jobs/tasks
    - These values should then be excluded from the config file
