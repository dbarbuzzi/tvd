# Todo

- [x] Refactor CLI API
  - [x] Consider use-cases (e.g. simplest use-case should allow accepting just a vod ID and sane defaults for *everything* else)
  - [x] Consider using a module (e.g. `kingpin`)
- [ ] Logging
  - [ ] File: Move to `$HOME/logs/tvd.log`
    - Exact folder/path may need adjustment
  - [ ] File: Create intermediate directories if they don’t exist
  - [ ] File: (?) Support parameter (CLI flag, config param) to specify location
  - [ ] Contents: Potential for additional events to be logged (spinning up workers, worker receiving a task, chunk download complete, worker completing a task)
- [ ] Config file
  - [ ] File: Default location should be inside folder `$HOME/.config/tvd`
  - [ ] File: Support parameter (CLI flag) to specify location
- [ ] Job file
  - [ ] Add support for "job" file which contains config for specific download jobs/tasks
    - These values should then be excluded from the config file
